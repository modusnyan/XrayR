// Package config loads, normalizes, validates, and migrates XrayR configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/common/limiter"
	"github.com/XrayR-project/XrayR/common/mylego"
	"github.com/XrayR-project/XrayR/panel"
	"github.com/XrayR-project/XrayR/service/controller"
)

const CurrentVersion = 1

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Issue struct {
	Severity   Severity `json:"severity" yaml:"severity"`
	Path       string   `json:"path" yaml:"path"`
	Message    string   `json:"message" yaml:"message"`
	Suggestion string   `json:"suggestion,omitempty" yaml:"suggestion,omitempty"`
}

type Result struct {
	Config *panel.Config `json:"-" yaml:"-"`
	Issues []Issue       `json:"issues" yaml:"issues"`
}

func (r Result) HasErrors() bool {
	for _, issue := range r.Issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r Result) Error() error {
	if !r.HasErrors() {
		return nil
	}
	return errors.New("configuration contains errors")
}

// Load reads a configuration file with strict unknown-field checking.
func Load(path string) (Result, error) {
	file, err := os.Open(path)
	if err != nil {
		// os.Open already reports the path and the reason; adding "open config %s"
		// on top produced "open config config.yml: open config.yml: no such file".
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("configuration file %s does not exist; pass -c to point at it, or run the installer to create it", path)
		}
		return Result{}, fmt.Errorf("configuration file %s is not readable: %w", path, err)
	}
	// The file is only read, so a close failure carries no information.
	defer func() { _ = file.Close() }()

	result, err := Decode(file)
	if err != nil {
		return Result{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	ApplyDefaults(result.Config, filepath.Dir(path))
	result.Issues = append(result.Issues, Validate(result.Config)...)
	return result, nil
}

// Decode parses YAML and turns recognized retired fields into migration warnings.
func Decode(reader io.Reader) (Result, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return Result{}, err
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Result{}, err
	}
	issues := removeDeprecatedFields(raw)

	// Only re-encode when something was actually removed. Re-marshalling sorts the keys
	// and rewrites the document, which makes every line number in a decode error point
	// at the rewritten YAML instead of the file the operator is looking at.
	cleaned := data
	if len(issues) > 0 {
		reencoded, err := yaml.Marshal(raw)
		if err != nil {
			return Result{}, err
		}
		cleaned = reencoded
	}

	cfg := new(panel.Config)
	decoder := yaml.NewDecoder(strings.NewReader(string(cleaned)))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return Result{}, fmt.Errorf("unknown or invalid field: %w", explainNesting(err))
	}
	return Result{Config: cfg, Issues: issues}, nil
}

// configLevel names one nesting level of the configuration. It exists so that a field
// written at the wrong depth can be reported with the depth it belongs to.
type configLevel struct {
	label string
	value interface{}
}

var configLevels = []configLevel{
	{"the top level", panel.Config{}},
	{"Nodes[]", panel.NodesConfig{}},
	{"Nodes[].ApiConfig", api.Config{}},
	{"Nodes[].ControllerConfig", controller.Config{}},
	{"Nodes[].ControllerConfig.CertConfig", mylego.CertConfig{}},
	{"Nodes[].ControllerConfig.REALITYConfigs", controller.REALITYConfig{}},
	{"Nodes[].ControllerConfig.FallBackConfigs[]", controller.FallBackConfig{}},
	{"Nodes[].ControllerConfig.AutoSpeedLimitConfig", controller.AutoSpeedLimitConfig{}},
	{"Nodes[].ControllerConfig.GlobalDeviceLimitConfig", limiter.GlobalDeviceLimitConfig{}},
}

func levelLabelForType(typeName string) string {
	short := typeName
	if index := strings.LastIndex(short, "."); index >= 0 {
		short = short[index+1:]
	}
	for _, level := range configLevels {
		if reflect.TypeOf(level.value).Name() == short {
			return level.label
		}
	}
	return ""
}

func levelLabelForField(field string) string {
	for _, level := range configLevels {
		typ := reflect.TypeOf(level.value)
		for i := 0; i < typ.NumField(); i++ {
			tag := typ.Field(i).Tag.Get("yaml")
			name, _, _ := strings.Cut(tag, ",")
			if name == field {
				return level.label
			}
		}
	}
	return ""
}

// explainNesting appends "it belongs under X" hints to yaml's "field not found" errors.
//
// yaml.v3 reports `field CertConfig not found in type panel.NodesConfig`, which tells
// the operator what is wrong but not where the field should go — CertConfig is valid,
// just one level deeper, under ControllerConfig.
func explainNesting(err error) error {
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return err
	}
	var hints []string
	for _, line := range typeErr.Errors {
		field, foundIn, ok := parseUnknownField(line)
		if !ok {
			continue
		}
		owner := levelLabelForField(field)
		where := levelLabelForType(foundIn)
		if owner == "" || owner == where {
			continue
		}
		hints = append(hints, fmt.Sprintf(
			"hint: %s is valid, but it does not belong under %s; nest it under %s",
			field, where, owner))
	}
	if len(hints) == 0 {
		return err
	}
	return fmt.Errorf("%w\n  %s", err, strings.Join(hints, "\n  "))
}

// parseUnknownField pulls the field name and the Go type out of a yaml error line such
// as `line 12: field CertConfig not found in type panel.NodesConfig`.
func parseUnknownField(line string) (field, foundIn string, ok bool) {
	const marker = "field "
	index := strings.Index(line, marker)
	if index < 0 {
		return "", "", false
	}
	rest := line[index+len(marker):]
	field, rest, found := strings.Cut(rest, " ")
	if !found {
		return "", "", false
	}
	_, after, found := strings.Cut(rest, "not found in type ")
	if !found {
		return "", "", false
	}
	foundIn = strings.TrimSpace(after)
	if field == "" || foundIn == "" {
		return "", "", false
	}
	return field, foundIn, true
}

func removeDeprecatedFields(raw map[string]interface{}) []Issue {
	var issues []Issue
	nodes, ok := lookup(raw, "Nodes").([]interface{})
	if !ok {
		return issues
	}
	for i, value := range nodes {
		node, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		apiConfig, ok := lookup(node, "ApiConfig").(map[string]interface{})
		if !ok {
			continue
		}
		if removeKey(apiConfig, "MachineID") {
			issues = append(issues, Issue{
				Severity:   SeverityWarning,
				Path:       fmt.Sprintf("Nodes[%d].ApiConfig.MachineID", i),
				Message:    "MachineID has been removed and will be ignored",
				Suggestion: "Xboard now uses the stable UniProxy REST adapter; remove MachineID",
			})
		}
	}
	return issues
}

func lookup(values map[string]interface{}, key string) interface{} {
	for candidate, value := range values {
		if strings.EqualFold(candidate, key) {
			return value
		}
	}
	return nil
}

func removeKey(values map[string]interface{}, key string) bool {
	for candidate := range values {
		if strings.EqualFold(candidate, key) {
			delete(values, candidate)
			return true
		}
	}
	return false
}

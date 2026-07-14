// Package config loads, normalizes, validates, and migrates XrayR configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/XrayR-project/XrayR/panel"
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
		return Result{}, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

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
	cleaned, err := yaml.Marshal(raw)
	if err != nil {
		return Result{}, err
	}

	cfg := new(panel.Config)
	decoder := yaml.NewDecoder(strings.NewReader(string(cleaned)))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return Result{}, fmt.Errorf("unknown or invalid field: %w", err)
	}
	return Result{Config: cfg, Issues: issues}, nil
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

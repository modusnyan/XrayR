package configui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"gopkg.in/yaml.v3"

	xconfig "github.com/XrayR-project/XrayR/config"
	"github.com/XrayR-project/XrayR/internal/redact"
	"github.com/XrayR-project/XrayR/panel"
	"github.com/XrayR-project/XrayR/preflight"
)

var (
	ErrCancelled    = errors.New("configuration cancelled")
	errReturnToEdit = errors.New("return to configuration editor")
)

type Options struct {
	Output     string
	Force      bool
	SkipVerify bool
	SkipDoctor bool
	Input      io.Reader
	Writer     io.Writer
}

func Run(existing *panel.Config, options Options) (*panel.Config, error) {
	if options.Output == "" {
		options.Output = "config.yml"
	}
	if options.Input == nil {
		options.Input = os.Stdin
	}
	if options.Writer == nil {
		options.Writer = os.Stdout
	}
	configDir := filepath.Dir(options.Output)
	state := NewState(configDir)
	if existing != nil {
		state = StateFromConfig(existing, configDir)
	}

	if err := runForm(options, globalGroups(state)...); err != nil {
		return nil, err
	}
	if existing == nil && len(state.Nodes) == 1 {
		if err := editNode(state, 0, options); err != nil {
			return nil, err
		}
	}
	for {
		action, index, err := nodeMenu(state, options)
		if err != nil {
			return nil, err
		}
		switch action {
		case "done":
			cfg, saveErr := reviewAndSave(state, existing, options)
			if errors.Is(saveErr, errReturnToEdit) {
				continue
			}
			return cfg, saveErr
		case "add":
			index = state.AddNode()
			if err := editNode(state, index, options); err != nil {
				return nil, err
			}
		case "edit":
			if err := editNode(state, index, options); err != nil {
				return nil, err
			}
		case "clone":
			if cloneIndex := state.CloneNode(index); cloneIndex >= 0 {
				if err := editNode(state, cloneIndex, options); err != nil {
					return nil, err
				}
			}
		case "delete":
			state.RemoveNode(index)
		}
	}
}

func runForm(options Options, groups ...*huh.Group) error {
	form := huh.NewForm(groups...).WithInput(options.Input).WithOutput(options.Writer).WithTheme(huh.ThemeBase16())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrCancelled
		}
		return err
	}
	return nil
}

func editNode(state *State, index int, options Options) error {
	if index < 0 || index >= len(state.Nodes) {
		return fmt.Errorf("node index %d is out of range", index)
	}
	for {
		node := &state.Nodes[index]
		if len(node.Fallbacks) == 0 {
			node.Fallbacks = append(node.Fallbacks, FallbackState{Dest: "80", ProxyProtocolVer: "0"})
		}
		if err := runForm(options, nodeGroups(node)...); err != nil {
			return err
		}
		if !node.EnableFallback || !supportsFallback(node.NodeType, node.EnableVless) {
			return nil
		}
		var action string
		choices := []huh.Option[string]{huh.NewOption("Finish this node", "done"), huh.NewOption("Add another fallback", "add")}
		if len(node.Fallbacks) > 1 {
			choices = append(choices, huh.NewOption("Remove the last fallback", "remove"))
		}
		if err := runForm(options, huh.NewGroup(huh.NewSelect[string]().Title("Fallback entries").Options(choices...).Value(&action))); err != nil {
			return err
		}
		switch action {
		case "add":
			node.Fallbacks = append(node.Fallbacks, FallbackState{Dest: "80", ProxyProtocolVer: "0"})
		case "remove":
			node.Fallbacks = node.Fallbacks[:len(node.Fallbacks)-1]
		default:
			return nil
		}
	}
}

func nodeMenu(state *State, options Options) (string, int, error) {
	choices := []huh.Option[string]{huh.NewOption("Review, validate and save", "done"), huh.NewOption("Add node", "add")}
	for index, node := range state.Nodes {
		choices = append(choices,
			huh.NewOption("Edit: "+nodeSummary(node, index), fmt.Sprintf("edit:%d", index)),
			huh.NewOption("Clone: "+nodeSummary(node, index), fmt.Sprintf("clone:%d", index)),
		)
		if len(state.Nodes) > 1 {
			choices = append(choices, huh.NewOption("Delete: "+nodeSummary(node, index), fmt.Sprintf("delete:%d", index)))
		}
	}
	var selected string
	if err := runForm(options, huh.NewGroup(
		huh.NewNote().Title("Configured nodes").Description("Add, edit, clone or delete nodes before review."),
		huh.NewSelect[string]().Title("Next action").Options(choices...).Value(&selected),
	)); err != nil {
		return "", -1, err
	}
	parts := strings.SplitN(selected, ":", 2)
	if len(parts) == 1 {
		return parts[0], -1, nil
	}
	var index int
	_, _ = fmt.Sscanf(parts[1], "%d", &index)
	return parts[0], index, nil
}

func reviewAndSave(state *State, existing *panel.Config, options Options) (*panel.Config, error) {
	cfg, err := state.Config(filepath.Dir(options.Output))
	if err != nil {
		return nil, err
	}
	issues := xconfig.Validate(cfg)
	validationResult := xconfig.Result{Issues: issues}
	redactedCfg := redact.Config(cfg)
	preview, err := yaml.Marshal(redactedCfg)
	if err != nil {
		return nil, err
	}

	var description strings.Builder
	description.WriteString(string(preview))
	if existing != nil {
		oldData, _ := yaml.Marshal(redact.Config(existing))
		description.WriteString("\n--- Redacted changes ---\n")
		description.WriteString(DiffText(string(oldData), string(preview)))
	}
	description.WriteString("\n--- Validation ---\n")
	if len(issues) == 0 {
		description.WriteString("OK: configuration is valid\n")
	} else {
		for _, issue := range issues {
			fmt.Fprintf(&description, "%s %s: %s", strings.ToUpper(string(issue.Severity)), issue.Path, issue.Message)
			if issue.Suggestion != "" {
				fmt.Fprintf(&description, " (%s)", issue.Suggestion)
			}
			description.WriteByte('\n')
		}
	}
	if validationResult.HasErrors() {
		_ = runForm(options, huh.NewGroup(huh.NewNote().Title("Configuration cannot be saved").Description(description.String()).NextLabel("Return to editor")))
		return nil, errReturnToEdit
	}

	runDoctor := !options.SkipDoctor
	var save bool
	fields := []huh.Field{huh.NewNote().Title("Review configuration").Description(description.String())}
	if !options.SkipDoctor {
		fields = append(fields, huh.NewConfirm().Title("Run Doctor before saving?").Affirmative("Run Doctor").Negative("Skip").Value(&runDoctor))
	}
	fields = append(fields, huh.NewConfirm().Title("Save this configuration?").Affirmative("Save").Negative("Cancel").Value(&save))
	if err := runForm(options, huh.NewGroup(fields...)); err != nil {
		return nil, err
	}
	if !save {
		return nil, ErrCancelled
	}

	if runDoctor && !options.SkipDoctor {
		results := preflight.Run(context.Background(), cfg, preflight.Options{Node: -1, Timeout: 5 * time.Second, Remote: !options.SkipVerify})
		var doctorOutput strings.Builder
		failed := false
		for _, result := range results {
			fmt.Fprintf(&doctorOutput, "%s [%s] %s: %s", strings.ToUpper(string(result.Status)), result.Section, result.Name, result.Detail)
			if result.Suggestion != "" {
				fmt.Fprintf(&doctorOutput, " (%s)", result.Suggestion)
			}
			doctorOutput.WriteByte('\n')
			if result.Status == preflight.StatusError {
				failed = true
			}
		}
		if failed {
			var saveAnyway bool
			if err := runForm(options, huh.NewGroup(
				huh.NewNote().Title("Doctor found errors").Description(doctorOutput.String()),
				huh.NewConfirm().Title("Save anyway for an offline or incomplete deployment?").Value(&saveAnyway),
			)); err != nil {
				return nil, err
			}
			if !saveAnyway {
				return nil, errReturnToEdit
			}
		} else {
			if err := runForm(options, huh.NewGroup(huh.NewNote().Title("Doctor checks passed").Description(doctorOutput.String()).NextLabel("Save"))); err != nil {
				return nil, err
			}
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	overwrite := options.Force || existing != nil
	if err := xconfig.WriteAtomic(options.Output, data, overwrite); err != nil {
		return nil, err
	}
	return cfg, nil
}

func DiffText(oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	var builder strings.Builder
	builder.WriteString("--- existing\n+++ generated\n")
	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}
	for i := 0; i < max; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine == newLine {
			continue
		}
		if i < len(oldLines) {
			fmt.Fprintf(&builder, "-%s\n", oldLine)
		}
		if i < len(newLines) {
			fmt.Fprintf(&builder, "+%s\n", newLine)
		}
	}
	return builder.String()
}

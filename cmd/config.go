package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	xconfig "github.com/XrayR-project/XrayR/config"
	"github.com/XrayR-project/XrayR/internal/redact"
	"github.com/XrayR-project/XrayR/panel"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Create, validate, display, and migrate configuration",
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(newConfigCheckCommand(), newConfigShowCommand(), newConfigMigrateCommand())
}

func newConfigCheckCommand() *cobra.Command {
	var format string
	command := &cobra.Command{
		Use:   "check",
		Short: "Validate configuration without contacting the panel",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := xconfig.Load(resolveConfigPath())
			if err != nil {
				return err
			}
			if err := printIssues(cmd.OutOrStdout(), format, result); err != nil {
				return err
			}
			if result.HasErrors() {
				return silentError{"configuration is invalid"}
			}
			return nil
		},
	}
	command.Flags().StringVarP(&format, "format", "o", "text", "Output format: text or json")
	return command
}

func newConfigShowCommand() *cobra.Command {
	var format string
	command := &cobra.Command{
		Use:   "show",
		Short: "Show normalized configuration with secrets redacted",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := xconfig.Load(resolveConfigPath())
			if err != nil {
				return err
			}
			if result.HasErrors() {
				_ = printIssues(cmd.ErrOrStderr(), "text", result)
				return silentError{"configuration is invalid"}
			}
			view := newConfigView(redact.Config(result.Config))
			switch strings.ToLower(format) {
			case "json":
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(view)
			case "yaml", "yml":
				encoder := yaml.NewEncoder(cmd.OutOrStdout())
				defer func() { _ = encoder.Close() }()
				return encoder.Encode(view)
			default:
				return fmt.Errorf("unsupported format %q; use yaml or json", format)
			}
		},
	}
	command.Flags().StringVarP(&format, "format", "o", "yaml", "Output format: yaml or json")
	return command
}

func newConfigMigrateCommand() *cobra.Command {
	var input, output string
	var force bool
	command := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate an older configuration to the current version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				input = resolveConfigPath()
			}
			if output == "" {
				return errors.New("--output is required")
			}
			file, err := os.Open(input)
			if err != nil {
				return err
			}
			defer func() { _ = file.Close() }()
			data, issues, err := xconfig.Migrate(file)
			if err != nil {
				return err
			}
			if err := xconfig.WriteAtomic(output, data, force); err != nil {
				if errors.Is(err, os.ErrExist) {
					return fmt.Errorf("output %s already exists; use --force to replace it", output)
				}
				return err
			}
			for _, issue := range issues {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s: %s\n", issue.Severity, issue.Path, issue.Message)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Migrated configuration written to %s\n", output)
			return nil
		},
	}
	command.Flags().StringVar(&input, "input", "", "Input configuration path")
	command.Flags().StringVar(&output, "output", "", "Output configuration path")
	command.Flags().BoolVar(&force, "force", false, "Replace an existing output file")
	return command
}

// installedConfigPath is where the installer puts the configuration and localConfigPath
// is the working-directory fallback used when running from a checkout.
const (
	installedConfigPath = "/etc/XrayR/config.yml"
	localConfigPath     = "config.yml"
)

// resolveConfigPath picks the configuration file to read. An explicit -c always wins;
// otherwise the installed location is preferred over the working directory, so running
// the binary bare on an installed node finds the config the service uses instead of
// failing with "open config.yml: no such file or directory".
func resolveConfigPath() string {
	if cfgFile != "" {
		return cfgFile
	}
	return firstExisting(installedConfigPath, localConfigPath)
}

// firstExisting returns the first candidate that exists. When none do it returns the
// last one, so the resulting error names the location an installed node would use.
func firstExisting(candidates ...string) string {
	for _, candidate := range candidates[:len(candidates)-1] {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[len(candidates)-1]
}

func printIssues(writer io.Writer, format string, result xconfig.Result) error {
	if strings.EqualFold(format, "json") {
		payload := struct {
			Valid  bool            `json:"valid"`
			Issues []xconfig.Issue `json:"issues"`
		}{Valid: !result.HasErrors(), Issues: result.Issues}
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(payload)
	}
	if !strings.EqualFold(format, "text") {
		return fmt.Errorf("unsupported format %q; use text or json", format)
	}
	if len(result.Issues) == 0 {
		_, _ = fmt.Fprintln(writer, "✓ Configuration is valid")
		return nil
	}
	for _, issue := range result.Issues {
		_, _ = fmt.Fprintf(writer, "%s: %s: %s\n", strings.ToUpper(string(issue.Severity)), issue.Path, issue.Message)
		if issue.Suggestion != "" {
			_, _ = fmt.Fprintf(writer, "  Suggestion: %s\n", issue.Suggestion)
		}
	}
	if !result.HasErrors() {
		_, _ = fmt.Fprintln(writer, "✓ Configuration is valid with warnings")
	}
	return nil
}

type configView struct {
	*panel.Config
	EffectiveAdapters map[int]string `json:"EffectiveAdapters" yaml:"EffectiveAdapters"`
}

func newConfigView(cfg *panel.Config) configView {
	adapters := make(map[int]string, len(cfg.NodesConfig))
	for i, node := range cfg.NodesConfig {
		if node != nil {
			if definition, err := panel.LookupPanel(node.PanelType); err == nil {
				adapters[i] = definition.Adapter
			}
		}
	}
	return configView{Config: cfg, EffectiveAdapters: adapters}
}

type silentError struct{ message string }

func (e silentError) Error() string { return e.message }

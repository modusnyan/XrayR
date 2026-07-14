package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/XrayR-project/XrayR/api"
	xconfig "github.com/XrayR-project/XrayR/config"
	"github.com/XrayR-project/XrayR/panel"
	"github.com/XrayR-project/XrayR/preflight"
)

func init() { configCmd.AddCommand(newConfigInitCommand()) }

func newConfigInitCommand() *cobra.Command {
	var panelName, apiHost, apiKey, nodeType, output string
	var nodeID int
	var force, skipVerify, skipDoctor bool
	command := &cobra.Command{
		Use:   "init",
		Short: "Interactively or non-interactively create a minimal configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(cmd.InOrStdin())
			interactive := panelName == "" || apiHost == "" || apiKey == "" || nodeID == 0 || nodeType == ""
			if interactive {
				var err error
				if panelName == "" {
					panelName, err = promptPanel(cmd.OutOrStdout(), reader)
					if err != nil {
						return err
					}
				}
				if apiHost == "" {
					apiHost, err = prompt(cmd.OutOrStdout(), reader, "Panel URL")
					if err != nil {
						return err
					}
				}
				if apiKey == "" {
					apiKey, err = promptSecret(cmd.OutOrStdout(), reader, "API key")
					if err != nil {
						return err
					}
				}
				if nodeID == 0 {
					value, readErr := prompt(cmd.OutOrStdout(), reader, "Node ID")
					if readErr != nil {
						return readErr
					}
					nodeID, err = strconv.Atoi(value)
					if err != nil {
						return errors.New("node ID must be an integer")
					}
				}
				if nodeType == "" {
					nodeType, err = prompt(cmd.OutOrStdout(), reader, "Node type (Vless/Vmess/Trojan/Shadowsocks)")
					if err != nil {
						return err
					}
				}
			}
			definition, err := panel.LookupPanel(panelName)
			if err != nil {
				return err
			}
			if !definition.SupportsNodeType(nodeType) {
				return fmt.Errorf("%s does not support node type %s", definition.Name, nodeType)
			}
			cfg := &panel.Config{ConfigVersion: xconfig.CurrentVersion, LogConfig: &panel.LogConfig{Level: "info", Format: "text"}, NodesConfig: []*panel.NodesConfig{{PanelType: definition.Name, ApiConfig: &api.Config{APIHost: apiHost, Key: apiKey, NodeID: nodeID, NodeType: nodeType, Timeout: 30}}}}
			configDir := "."
			if output != "" {
				configDir = filepath.Dir(output)
			}
			xconfig.ApplyDefaults(cfg, configDir)
			for _, issue := range xconfig.Validate(cfg) {
				if issue.Severity == xconfig.SeverityError {
					return fmt.Errorf("%s: %s", issue.Path, issue.Message)
				}
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			if output == "" {
				output = resolveConfigPath()
			}
			if !force {
				if _, statErr := os.Stat(output); statErr == nil {
					if !interactive {
						return fmt.Errorf("output %s exists; use --force", output)
					}
					fmt.Fprintln(cmd.OutOrStdout(), DiffText(readExisting(output), string(data)))
					answer, askErr := prompt(cmd.OutOrStdout(), reader, "Overwrite? [y/N]")
					if askErr != nil {
						return askErr
					}
					if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
						return errors.New("cancelled")
					}
					force = true
				}
			}
			if err := xconfig.WriteAtomic(output, data, force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Configuration written to %s\n", output)
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Local validation passed")
			if !skipVerify || !skipDoctor {
				results := preflight.Run(context.Background(), cfg, preflight.Options{Node: -1, Timeout: 5 * time.Second, Remote: !skipVerify})
				failed := false
				for _, result := range results {
					if result.Status == preflight.StatusError {
						failed = true
					}
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s\n", result.Status, result.Name, result.Detail)
				}
				if failed {
					return errors.New("configuration was written, but remote verification failed; run XrayR doctor for details")
				}
				fmt.Fprintln(cmd.OutOrStdout(), "✓ Doctor checks passed")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Next: systemctl enable --now XrayR (config: %s)\n", output)
			return nil
		},
	}
	flags := command.Flags()
	flags.StringVar(&panelName, "panel", "", "Panel type")
	flags.StringVar(&apiHost, "api-host", "", "Panel URL")
	flags.StringVar(&apiKey, "api-key", "", "Panel API key")
	flags.IntVar(&nodeID, "node-id", 0, "Node ID")
	flags.StringVar(&nodeType, "node-type", "", "Node protocol")
	flags.StringVarP(&output, "output", "o", "", "Output path")
	flags.BoolVar(&force, "force", false, "Overwrite output")
	flags.BoolVar(&skipVerify, "skip-verify", false, "Skip remote verification")
	flags.BoolVar(&skipDoctor, "skip-doctor", false, "Skip doctor after writing")
	return command
}

func promptPanel(writer io.Writer, reader *bufio.Reader) (string, error) {
	definitions := panel.Panels()
	fmt.Fprintln(writer, "Supported panels:")
	for i, definition := range definitions {
		fmt.Fprintf(writer, "  %d. %s\n", i+1, definition.Name)
	}
	value, err := prompt(writer, reader, "Panel")
	if err != nil {
		return "", err
	}
	if index, parseErr := strconv.Atoi(value); parseErr == nil && index > 0 && index <= len(definitions) {
		return definitions[index-1].Name, nil
	}
	return value, nil
}
func prompt(writer io.Writer, reader *bufio.Reader, label string) (string, error) {
	fmt.Fprintf(writer, "%s: ", label)
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return "", err
	}
	return strings.TrimSpace(value), nil
}
func promptSecret(writer io.Writer, reader *bufio.Reader, label string) (string, error) {
	fmt.Fprintf(writer, "%s: ", label)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(writer)
		return strings.TrimSpace(string(value)), err
	}
	return prompt(writer, reader, "")
}
func readExisting(path string) string { data, _ := os.ReadFile(path); return string(data) }

// DiffText produces a deterministic line diff without extra dependencies.
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

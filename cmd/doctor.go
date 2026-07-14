package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	xconfig "github.com/XrayR-project/XrayR/config"
	"github.com/XrayR-project/XrayR/preflight"
)

func init() { rootCmd.AddCommand(newDoctorCommand()) }

func newDoctorCommand() *cobra.Command {
	var format string
	var node int
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Run read-only configuration, environment, and panel checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := xconfig.Load(resolveConfigPath())
			if err != nil {
				return err
			}
			results := preflight.Run(context.Background(), loaded.Config, preflight.Options{Node: node, Timeout: timeout, Remote: true})
			failed := false
			for _, result := range results {
				if result.Status == preflight.StatusError {
					failed = true
				}
			}
			if strings.EqualFold(format, "json") {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(results); err != nil {
					return err
				}
			} else if strings.EqualFold(format, "text") {
				for _, result := range results {
					symbol := "✓"
					if result.Status == preflight.StatusError {
						symbol = "✗"
					} else if result.Status == preflight.StatusWarning {
						symbol = "!"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s [%s] %s: %s\n", symbol, result.Section, result.Name, result.Detail)
					if result.Suggestion != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  Suggestion: %s\n", result.Suggestion)
					}
				}
			} else {
				return fmt.Errorf("unsupported format %q; use text or json", format)
			}
			if failed {
				return silentError{"doctor found errors"}
			}
			return nil
		},
	}
	command.Flags().StringVarP(&format, "format", "o", "text", "Output format: text or json")
	command.Flags().IntVar(&node, "node", -1, "Only check a zero-based node index")
	command.Flags().DurationVar(&timeout, "timeout", 5*time.Second, "Timeout for each network check")
	return command
}

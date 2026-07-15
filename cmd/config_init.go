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
	"github.com/XrayR-project/XrayR/common/mylego"
	"github.com/XrayR-project/XrayR/panel"
	"github.com/XrayR-project/XrayR/preflight"
	"github.com/XrayR-project/XrayR/service/controller"
)

func init() { configCmd.AddCommand(newConfigInitCommand()) }

func newConfigInitCommand() *cobra.Command {
	var panelName, apiHost, apiKey, nodeType, output string
	var nodeID int
	var force, skipVerify, skipDoctor bool

	// Certificate flags
	var certMode, certDomain, certProvider, certEmail, certFile, certKeyFile string
	var certRejectUnknownSni bool

	// REALITY flags
	var enableReality, realityShow bool
	var realityDest, realityServerNames, realityPrivateKey, realityShortIds string
	var realityMinClientVer, realityMaxClientVer string
	var realityMaxTimeDiff uint64

	command := &cobra.Command{
		Use:   "init",
		Short: "Interactively or non-interactively create a configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(cmd.InOrStdin())
			interactive := panelName == "" || apiHost == "" || apiKey == "" || nodeID == 0 || nodeType == ""

			// ---- Step 1: Panel & API ----
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

				// ---- Step 2: Certificate ----
				certMode, err = promptCertSection(cmd.OutOrStdout(), reader, certMode, certDomain, certProvider, certEmail, certFile, certKeyFile, certRejectUnknownSni)
				if err != nil {
					return err
				}
				if certMode != "" {
					switch certMode {
					case "dns", "http", "tls":
						if certDomain != "" {
							certDomain, _ = promptDefault(cmd.OutOrStdout(), reader, "  Domain", certDomain)
						} else {
							certDomain, err = prompt(cmd.OutOrStdout(), reader, "  Domain")
							if err != nil {
								return err
							}
						}
						if certProvider != "" {
							certProvider, _ = promptDefault(cmd.OutOrStdout(), reader, "  DNS provider (e.g. alidns, cloudflare)", certProvider)
						} else {
							certProvider, err = promptDefault(cmd.OutOrStdout(), reader, "  DNS provider (e.g. alidns, cloudflare)", "alidns")
							if err != nil {
								return err
							}
						}
						if certEmail != "" {
							certEmail, _ = promptDefault(cmd.OutOrStdout(), reader, "  Email for Let's Encrypt", certEmail)
						} else {
							certEmail, err = prompt(cmd.OutOrStdout(), reader, "  Email for Let's Encrypt")
							if err != nil {
								return err
							}
						}
					case "file":
						if certFile != "" {
							certFile, _ = promptDefault(cmd.OutOrStdout(), reader, "  Certificate file path", certFile)
						} else {
							certFile, err = prompt(cmd.OutOrStdout(), reader, "  Certificate file path")
							if err != nil {
								return err
							}
						}
						if certKeyFile != "" {
							certKeyFile, _ = promptDefault(cmd.OutOrStdout(), reader, "  Private key file path", certKeyFile)
						} else {
							certKeyFile, err = prompt(cmd.OutOrStdout(), reader, "  Private key file path")
							if err != nil {
								return err
							}
						}
					case "none":
						// nothing extra
					}
				}

				// ---- Step 3: REALITY ----
				enableReality, realityShow, realityDest, realityServerNames, realityPrivateKey, realityShortIds,
					realityMinClientVer, realityMaxClientVer, realityMaxTimeDiff, err =
					promptRealitySection(cmd.OutOrStdout(), reader,
						enableReality, realityShow, realityDest, realityServerNames,
						realityPrivateKey, realityShortIds,
						realityMinClientVer, realityMaxClientVer, realityMaxTimeDiff)
				if err != nil {
					return err
				}
			}

			// ---- Build config ----
			definition, err := panel.LookupPanel(panelName)
			if err != nil {
				return err
			}
			if !definition.SupportsNodeType(nodeType) {
				return fmt.Errorf("%s does not support node type %s", definition.Name, nodeType)
			}

			nodeConfig := &panel.NodesConfig{
				PanelType: definition.Name,
				ApiConfig: &api.Config{
					APIHost: apiHost, Key: apiKey, NodeID: nodeID, NodeType: nodeType, Timeout: 30,
				},
			}

			// Build ControllerConfig with cert and REALITY
			ctrlCfg := &controller.Config{}
			if certMode != "" && certMode != "none" {
				ctrlCfg.CertConfig = &mylego.CertConfig{
					CertMode:         certMode,
					CertDomain:       certDomain,
					CertFile:         certFile,
					KeyFile:          certKeyFile,
					Provider:         certProvider,
					Email:            certEmail,
					RejectUnknownSni: certRejectUnknownSni,
				}
				if certMode == "dns" || certMode == "http" || certMode == "tls" {
					if certDomain != "" {
						ctrlCfg.CertConfig.CertDomain = certDomain
					}
					if certProvider != "" {
						ctrlCfg.CertConfig.Provider = certProvider
					}
				}
			}
			if enableReality {
				serverNames := strings.Split(realityServerNames, ",")
				for i := range serverNames {
					serverNames[i] = strings.TrimSpace(serverNames[i])
				}
				shortIds := strings.Split(realityShortIds, ",")
				for i := range shortIds {
					shortIds[i] = strings.TrimSpace(shortIds[i])
				}
				if realityShortIds == "" {
					shortIds = []string{""}
				}
				ctrlCfg.EnableREALITY = true
				ctrlCfg.REALITYConfigs = &controller.REALITYConfig{
					Show:         realityShow,
					Dest:         realityDest,
					ServerNames:  serverNames,
					PrivateKey:   realityPrivateKey,
					ShortIds:     shortIds,
					MinClientVer: realityMinClientVer,
					MaxClientVer: realityMaxClientVer,
					MaxTimeDiff:  realityMaxTimeDiff,
				}
			}
			nodeConfig.ControllerConfig = ctrlCfg

			cfg := &panel.Config{
				ConfigVersion: xconfig.CurrentVersion,
				LogConfig:     &panel.LogConfig{Level: "info", Format: "text"},
				NodesConfig:   []*panel.NodesConfig{nodeConfig},
			}
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

	// Certificate flags
	flags.StringVar(&certMode, "cert-mode", "", "TLS certificate mode (none/file/http/tls/dns)")
	flags.StringVar(&certDomain, "cert-domain", "", "Certificate domain")
	flags.StringVar(&certProvider, "cert-provider", "", "DNS provider for ACME challenge")
	flags.StringVar(&certEmail, "cert-email", "", "Email for Let's Encrypt")
	flags.StringVar(&certFile, "cert-file", "", "Path to TLS certificate file (mode=file)")
	flags.StringVar(&certKeyFile, "cert-key-file", "", "Path to TLS private key file (mode=file)")
	flags.BoolVar(&certRejectUnknownSni, "cert-reject-unknown-sni", false, "Reject unknown SNI in TLS")

	// REALITY flags
	flags.BoolVar(&enableReality, "enable-reality", false, "Enable REALITY")
	flags.BoolVar(&realityShow, "reality-show", true, "Show REALITY debug info")
	flags.StringVar(&realityDest, "reality-dest", "www.amazon.com:443", "REALITY destination address")
	flags.StringVar(&realityServerNames, "reality-server-names", "www.amazon.com", "REALITY server names (comma-separated)")
	flags.StringVar(&realityPrivateKey, "reality-private-key", "", "REALITY private key")
	flags.StringVar(&realityShortIds, "reality-short-ids", "", "REALITY short IDs (comma-separated)")
	flags.StringVar(&realityMinClientVer, "reality-min-client-ver", "", "REALITY minimum client version")
	flags.StringVar(&realityMaxClientVer, "reality-max-client-ver", "", "REALITY maximum client version")
	flags.Uint64Var(&realityMaxTimeDiff, "reality-max-time-diff", 0, "REALITY max time diff (ms)")

	return command
}

// promptCertSection handles the interactive TLS certificate configuration.
func promptCertSection(writer io.Writer, reader *bufio.Reader, certMode, certDomain, certProvider, certEmail, certFile, certKeyFile string, certRejectUnknownSni bool) (string, error) {
	if certMode != "" {
		return certMode, nil // already set via flags
	}
	answer, err := promptYN(writer, reader, "Configure HTTPS/TLS certificate? (recommended for production)", false)
	if err != nil {
		return "", err
	}
	if !answer {
		return "", nil
	}

	fmt.Fprintln(writer, "  Certificate mode:")
	fmt.Fprintln(writer, "    1. dns    (DNS-01 challenge, recommended)")
	fmt.Fprintln(writer, "    2. http   (HTTP-01 challenge, needs port 80)")
	fmt.Fprintln(writer, "    3. tls    (TLS-ALPN-01 challenge)")
	fmt.Fprintln(writer, "    4. file   (use existing certificate files)")
	fmt.Fprintln(writer, "    5. none   (no certificate)")
	modeStr, err := promptDefault(writer, reader, "  Choice", "1")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(modeStr) {
	case "1":
		certMode = "dns"
	case "2":
		certMode = "http"
	case "3":
		certMode = "tls"
	case "4":
		certMode = "file"
	case "5":
		certMode = "none"
	default:
		certMode = "dns"
	}
	return certMode, nil
}

// promptRealitySection handles the interactive REALITY configuration.
func promptRealitySection(writer io.Writer, reader *bufio.Reader,
	enableReality, realityShow bool,
	realityDest, realityServerNames, realityPrivateKey, realityShortIds string,
	realityMinClientVer, realityMaxClientVer string, realityMaxTimeDiff uint64,
) (bool, bool, string, string, string, string, string, string, uint64, error) {

	if enableReality {
		// already set via flags
		if realityDest == "" {
			realityDest = "www.amazon.com:443"
		}
		if realityServerNames == "" {
			realityServerNames = "www.amazon.com"
		}
		return enableReality, realityShow, realityDest, realityServerNames, realityPrivateKey,
			realityShortIds, realityMinClientVer, realityMaxClientVer, realityMaxTimeDiff, nil
	}

	answer, err := promptYN(writer, reader, "Enable REALITY? (requires Xray-core >= 1.8.0)", false)
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}
	if !answer {
		return false, false, "", "", "", "", "", "", 0, nil
	}

	show, err := promptYN(writer, reader, "  Show REALITY debug info?", true)
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	dest, err := promptDefault(writer, reader, "  Destination", "www.amazon.com:443")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	serverNames, err := promptDefault(writer, reader, "  Server names (comma-separated)", "www.amazon.com")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	privateKey, err := promptDefault(writer, reader, "  Private key (leave empty to skip)", "")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	shortIds, err := promptDefault(writer, reader, "  Short IDs (comma-separated, e.g. abc123,def456)", "")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	minVer, err := promptDefault(writer, reader, "  Min client version (optional)", "")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	maxVer, err := promptDefault(writer, reader, "  Max client version (optional)", "")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	return true, show, dest, serverNames, privateKey, shortIds, minVer, maxVer, 0, nil
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

// promptDefault prompts with a default value shown in brackets.
func promptDefault(writer io.Writer, reader *bufio.Reader, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Fprintf(writer, "%s [%s]: ", label, defaultVal)
	} else {
		fmt.Fprintf(writer, "%s: ", label)
	}
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return defaultVal, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultVal, nil
	}
	return value, nil
}

// promptYN asks a yes/no question and returns the boolean answer.
func promptYN(writer io.Writer, reader *bufio.Reader, label string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	fallback := false
	if defaultYes {
		suffix = "[Y/n]"
		fallback = true
	}
	fmt.Fprintf(writer, "%s %s: ", label, suffix)
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return fallback, err
	}
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback, nil
	}
	if value == "y" || value == "yes" {
		return true, nil
	}
	if value == "n" || value == "no" {
		return false, nil
	}
	return fallback, nil
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

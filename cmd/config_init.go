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
	"github.com/XrayR-project/XrayR/common/limiter"
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

	// Node-specific
	var enableVless bool
	var vlessFlow string

	// Certificate
	var certMode, certDomain, certProvider, certEmail, certFile, certKeyFile string
	var certDNSEnvStr string
	var certRejectUnknownSni bool

	// REALITY
	var enableReality, realityShow bool
	var realityDest, realityServerNames, realityPrivateKey, realityShortIds string
	var realityMinClientVer, realityMaxClientVer string
	var realityMaxTimeDiff uint64

	// Network & rate limits
	var listenIP, sendIP string
	var speedLimit float64
	var deviceLimit int

	// Redis global device limit
	var redisEnable bool
	var redisAddr, redisPassword string
	var redisDB int

	// Advanced
	var enableProxyProtocol bool
	var enableFallback bool
	var fallbackSNI, fallbackAlpn, fallbackPath, fallbackDest string
	var ruleListPath string
	var disableUploadTraffic, disableGetRule bool

	command := &cobra.Command{
		Use:   "init",
		Short: "Interactively or non-interactively create a configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(cmd.InOrStdin())
			w := cmd.OutOrStdout()
			interactive := panelName == "" || apiHost == "" || apiKey == "" || nodeID == 0 || nodeType == ""

			if interactive {
				var err error

				// ---- Step 1: Panel & API ----
				if panelName == "" {
					panelName, err = promptPanel(w, reader)
					if err != nil {
						return err
					}
				}
				if apiHost == "" {
					apiHost, err = prompt(w, reader, "Panel URL")
					if err != nil {
						return err
					}
				}
				if apiKey == "" {
					apiKey, err = promptSecret(w, reader, "API key")
					if err != nil {
						return err
					}
				}
				if nodeID == 0 {
					value, readErr := prompt(w, reader, "Node ID")
					if readErr != nil {
						return readErr
					}
					nodeID, err = strconv.Atoi(value)
					if err != nil {
						return errors.New("node ID must be an integer")
					}
				}
				if nodeType == "" {
					nodeType, err = prompt(w, reader, "Node type (Vless/Vmess/Trojan/Shadowsocks)")
					if err != nil {
						return err
					}
				}

				// ---- Step 2: Node-specific options ----
				normalizedType := strings.ToLower(strings.TrimSpace(nodeType))
				if normalizedType == "v2ray" && !enableVless {
					enableVless, err = promptYN(w, reader, "Enable VLESS for V2ray node?", false)
					if err != nil {
						return err
					}
				}
				if enableVless || normalizedType == "vless" {
					if vlessFlow != "" {
						vlessFlow, _ = promptDefault(w, reader, "  VLESS flow", vlessFlow)
					} else {
						vlessFlow, err = promptDefault(w, reader, "  VLESS flow", "xtls-rprx-vision")
						if err != nil {
							return err
						}
					}
				}

				// ---- Step 3: Certificate ----
				certMode, err = promptCertSection(w, reader, certMode)
				if err != nil {
					return err
				}
				if certMode != "" && certMode != "none" {
					switch certMode {
					case "dns", "http", "tls":
						if certDomain != "" {
							certDomain, _ = promptDefault(w, reader, "  Domain", certDomain)
						} else {
							certDomain, err = prompt(w, reader, "  Domain")
							if err != nil {
								return err
							}
						}
						if certProvider != "" {
							certProvider, _ = promptDefault(w, reader, "  DNS provider (e.g. alidns, cloudflare)", certProvider)
						} else {
							certProvider, err = promptDefault(w, reader, "  DNS provider (e.g. alidns, cloudflare)", "alidns")
							if err != nil {
								return err
							}
						}
						if certEmail != "" {
							certEmail, _ = promptDefault(w, reader, "  Email for Let's Encrypt", certEmail)
						} else {
							certEmail, err = prompt(w, reader, "  Email for Let's Encrypt")
							if err != nil {
								return err
							}
						}
						// DNS environment variables
						if certDNSEnvStr == "" {
							certDNSEnvStr, err = promptDNSEnv(w, reader)
							if err != nil {
								return err
							}
						}
					case "file":
						if certFile != "" {
							certFile, _ = promptDefault(w, reader, "  Certificate file path", certFile)
						} else {
							certFile, err = prompt(w, reader, "  Certificate file path")
							if err != nil {
								return err
							}
						}
						if certKeyFile != "" {
							certKeyFile, _ = promptDefault(w, reader, "  Private key file path", certKeyFile)
						} else {
							certKeyFile, err = prompt(w, reader, "  Private key file path")
							if err != nil {
								return err
							}
						}
					}
				}

				// ---- Step 4: REALITY ----
				enableReality, realityShow, realityDest, realityServerNames, realityPrivateKey, realityShortIds,
					realityMinClientVer, realityMaxClientVer, realityMaxTimeDiff, err =
					promptRealitySection(w, reader,
						enableReality, realityShow, realityDest, realityServerNames,
						realityPrivateKey, realityShortIds,
						realityMinClientVer, realityMaxClientVer, realityMaxTimeDiff)
				if err != nil {
					return err
				}

				// ---- Step 5: Listen & rate limits ----
				fmt.Fprintln(w, "\n--- Network & Rate Limits ---")
				if listenIP != "" {
					listenIP, _ = promptDefault(w, reader, "  Listen IP", listenIP)
				} else {
					listenIP, err = promptDefault(w, reader, "  Listen IP", "0.0.0.0")
					if err != nil {
						return err
					}
				}
				if sendIP != "" {
					sendIP, _ = promptDefault(w, reader, "  Send IP (outbound)", sendIP)
				} else {
					sendIP, err = promptDefault(w, reader, "  Send IP (outbound)", "0.0.0.0")
					if err != nil {
						return err
					}
				}
				if speedLimit == 0 {
					spd, spdErr := promptDefault(w, reader, "  Speed limit (Mbps, 0=disable)", "0")
					if spdErr != nil {
						return spdErr
					}
					speedLimit, _ = strconv.ParseFloat(spd, 64)
				}
				if deviceLimit == 0 {
					dev, devErr := promptDefault(w, reader, "  Device limit (0=disable)", "0")
					if devErr != nil {
						return devErr
					}
					deviceLimit, _ = strconv.Atoi(dev)
				}

				// ---- Step 6: Redis global device limit ----
				redisEnable, err = promptYN(w, reader, "  Enable Redis global device limit?", false)
				if err != nil {
					return err
				}
				if redisEnable {
					if redisAddr != "" {
						redisAddr, _ = promptDefault(w, reader, "    Redis address", redisAddr)
					} else {
						redisAddr, err = promptDefault(w, reader, "    Redis address", "127.0.0.1:6379")
						if err != nil {
							return err
						}
					}
					if redisPassword != "" {
						redisPassword, _ = promptSecret(w, reader, "    Redis password")
					} else {
						redisPassword, err = promptDefault(w, reader, "    Redis password (optional)", "")
						if err != nil {
							return err
						}
					}
					if redisDB == 0 {
						dbStr, dbErr := promptDefault(w, reader, "    Redis DB", "0")
						if dbErr != nil {
							return dbErr
						}
						redisDB, _ = strconv.Atoi(dbStr)
					}
				}

				// ---- Step 7: Advanced ----
				doAdvanced, advErr := promptYN(w, reader, "Configure advanced options? (proxy protocol, fallback, rule list)", false)
				if advErr != nil {
					return advErr
				}
				if doAdvanced {
					enableProxyProtocol, err = promptYN(w, reader, "  Enable proxy protocol? (for CDN/nginx)", false)
					if err != nil {
						return err
					}
					if normalizedType == "trojan" || normalizedType == "vless" {
						enableFallback, err = promptYN(w, reader, "  Enable fallback? (Trojan/VLESS only)", false)
						if err != nil {
							return err
						}
						if enableFallback {
							fallbackDest, err = promptDefault(w, reader, "    Fallback destination (port)", "80")
							if err != nil {
								return err
							}
							fallbackSNI, err = promptDefault(w, reader, "    Fallback SNI (optional)", "")
							if err != nil {
								return err
							}
							fallbackAlpn, err = promptDefault(w, reader, "    Fallback ALPN (optional)", "")
							if err != nil {
								return err
							}
							fallbackPath, err = promptDefault(w, reader, "    Fallback HTTP path (optional)", "")
							if err != nil {
								return err
							}
						}
					}
					ruleListPath, err = promptDefault(w, reader, "  Rule list file path (optional)", "")
					if err != nil {
						return err
					}
					disableUploadTraffic, err = promptYN(w, reader, "  Disable traffic upload to panel?", false)
					if err != nil {
						return err
					}
					disableGetRule, err = promptYN(w, reader, "  Disable rule fetch from panel?", false)
					if err != nil {
						return err
					}
				}
			}

			// ==================== Build config ====================
			definition, panelErr := panel.LookupPanel(panelName)
			if panelErr != nil {
				return panelErr
			}
			if !definition.SupportsNodeType(nodeType) {
				return fmt.Errorf("%s does not support node type %s", definition.Name, nodeType)
			}

			apiCfg := &api.Config{
				APIHost:             apiHost,
				Key:                 apiKey,
				NodeID:              nodeID,
				NodeType:            nodeType,
				Timeout:             30,
				EnableVless:         enableVless,
				VlessFlow:           vlessFlow,
				SpeedLimit:          speedLimit,
				DeviceLimit:         deviceLimit,
				RuleListPath:        ruleListPath,
			}

			ctrlCfg := &controller.Config{
				ListenIP:             listenIP,
				SendIP:               sendIP,
				DisableUploadTraffic: disableUploadTraffic,
				DisableGetRule:       disableGetRule,
				EnableProxyProtocol:  enableProxyProtocol,
			}

			// Certificate
			if certMode != "" && certMode != "none" {
				certCfg := &mylego.CertConfig{
					CertMode:         certMode,
					CertDomain:       certDomain,
					CertFile:         certFile,
					KeyFile:          certKeyFile,
					Provider:         certProvider,
					Email:            certEmail,
					RejectUnknownSni: certRejectUnknownSni,
				}
				if certDNSEnvStr != "" {
					certCfg.DNSEnv = parseDNSEnv(certDNSEnvStr)
				}
				ctrlCfg.CertConfig = certCfg
			}

			// REALITY
			if enableReality {
				ctrlCfg.EnableREALITY = true
				ctrlCfg.REALITYConfigs = &controller.REALITYConfig{
					Show:         realityShow,
					Dest:         realityDest,
					ServerNames:  splitTrim(realityServerNames),
					PrivateKey:   realityPrivateKey,
					ShortIds:     splitTrim(realityShortIds),
					MinClientVer: realityMinClientVer,
					MaxClientVer: realityMaxClientVer,
					MaxTimeDiff:  realityMaxTimeDiff,
				}
				if realityShortIds == "" {
					ctrlCfg.REALITYConfigs.ShortIds = []string{""}
				}
			}

			// Fallback
			if enableFallback {
				ctrlCfg.EnableFallback = true
				ctrlCfg.FallBackConfigs = []*controller.FallBackConfig{{
					SNI:  fallbackSNI,
					Alpn: fallbackAlpn,
					Path: fallbackPath,
					Dest: fallbackDest,
				}}
			}

			// Redis global device limit
			if redisEnable {
				ctrlCfg.GlobalDeviceLimitConfig = &limiter.GlobalDeviceLimitConfig{
					Enable:        true,
					RedisNetwork:  "tcp",
					RedisAddr:     redisAddr,
					RedisPassword: redisPassword,
					RedisDB:       redisDB,
					Timeout:       5,
					Expiry:        60,
				}
			}

			nodeConfig := &panel.NodesConfig{
				PanelType:        definition.Name,
				ApiConfig:        apiCfg,
				ControllerConfig: ctrlCfg,
			}

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
					fmt.Fprintln(w, DiffText(readExisting(output), string(data)))
					answer, askErr := prompt(w, reader, "Overwrite? [y/N]")
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
			fmt.Fprintf(w, "Configuration written to %s\n", output)
			fmt.Fprintln(w, "✓ Local validation passed")
			if !skipVerify || !skipDoctor {
				results := preflight.Run(context.Background(), cfg, preflight.Options{Node: -1, Timeout: 5 * time.Second, Remote: !skipVerify})
				failed := false
				for _, result := range results {
					if result.Status == preflight.StatusError {
						failed = true
					}
					fmt.Fprintf(w, "[%s] %s: %s\n", result.Status, result.Name, result.Detail)
				}
				if failed {
					return errors.New("configuration was written, but remote verification failed; run XrayR doctor for details")
				}
				fmt.Fprintln(w, "✓ Doctor checks passed")
			}
			fmt.Fprintf(w, "Next: systemctl enable --now XrayR (config: %s)\n", output)
			return nil
		},
	}

	flags := command.Flags()
	// Panel & API
	flags.StringVar(&panelName, "panel", "", "Panel type")
	flags.StringVar(&apiHost, "api-host", "", "Panel URL")
	flags.StringVar(&apiKey, "api-key", "", "Panel API key")
	flags.IntVar(&nodeID, "node-id", 0, "Node ID")
	flags.StringVar(&nodeType, "node-type", "", "Node protocol")
	flags.StringVarP(&output, "output", "o", "", "Output path")
	flags.BoolVar(&force, "force", false, "Overwrite output")
	flags.BoolVar(&skipVerify, "skip-verify", false, "Skip remote verification")
	flags.BoolVar(&skipDoctor, "skip-doctor", false, "Skip doctor after writing")
	// Node-specific
	flags.BoolVar(&enableVless, "enable-vless", false, "Enable VLESS for V2ray nodes")
	flags.StringVar(&vlessFlow, "vless-flow", "", "VLESS flow (e.g. xtls-rprx-vision)")
	// Certificate
	flags.StringVar(&certMode, "cert-mode", "", "TLS certificate mode (none/file/http/tls/dns)")
	flags.StringVar(&certDomain, "cert-domain", "", "Certificate domain")
	flags.StringVar(&certProvider, "cert-provider", "", "DNS provider for ACME challenge")
	flags.StringVar(&certEmail, "cert-email", "", "Email for Let's Encrypt")
	flags.StringVar(&certFile, "cert-file", "", "Path to TLS certificate file (mode=file)")
	flags.StringVar(&certKeyFile, "cert-key-file", "", "Path to TLS private key file (mode=file)")
	flags.StringVar(&certDNSEnvStr, "cert-dns-env", "", "DNS env vars (KEY1=VAL1,KEY2=VAL2)")
	flags.BoolVar(&certRejectUnknownSni, "cert-reject-unknown-sni", false, "Reject unknown SNI in TLS")
	// REALITY
	flags.BoolVar(&enableReality, "enable-reality", false, "Enable REALITY")
	flags.BoolVar(&realityShow, "reality-show", true, "Show REALITY debug info")
	flags.StringVar(&realityDest, "reality-dest", "www.amazon.com:443", "REALITY destination address")
	flags.StringVar(&realityServerNames, "reality-server-names", "www.amazon.com", "REALITY server names (comma-separated)")
	flags.StringVar(&realityPrivateKey, "reality-private-key", "", "REALITY private key")
	flags.StringVar(&realityShortIds, "reality-short-ids", "", "REALITY short IDs (comma-separated)")
	flags.StringVar(&realityMinClientVer, "reality-min-client-ver", "", "REALITY minimum client version")
	flags.StringVar(&realityMaxClientVer, "reality-max-client-ver", "", "REALITY maximum client version")
	flags.Uint64Var(&realityMaxTimeDiff, "reality-max-time-diff", 0, "REALITY max time diff (ms)")
	// Network & rate limits
	flags.StringVar(&listenIP, "listen-ip", "", "Listen IP address")
	flags.StringVar(&sendIP, "send-ip", "", "Outbound IP address")
	flags.Float64Var(&speedLimit, "speed-limit", 0, "Speed limit in Mbps")
	flags.IntVar(&deviceLimit, "device-limit", 0, "Device limit (0=disable)")
	// Redis
	flags.BoolVar(&redisEnable, "redis-enable", false, "Enable Redis global device limit")
	flags.StringVar(&redisAddr, "redis-addr", "", "Redis address (host:port)")
	flags.StringVar(&redisPassword, "redis-password", "", "Redis password")
	flags.IntVar(&redisDB, "redis-db", 0, "Redis DB number")
	// Advanced
	flags.BoolVar(&enableProxyProtocol, "enable-proxy-protocol", false, "Enable proxy protocol")
	flags.BoolVar(&enableFallback, "enable-fallback", false, "Enable fallback")
	flags.StringVar(&fallbackDest, "fallback-dest", "", "Fallback destination port")
	flags.StringVar(&fallbackSNI, "fallback-sni", "", "Fallback SNI")
	flags.StringVar(&fallbackAlpn, "fallback-alpn", "", "Fallback ALPN")
	flags.StringVar(&fallbackPath, "fallback-path", "", "Fallback HTTP path")
	flags.StringVar(&ruleListPath, "rule-list-path", "", "Path to rule list file")
	flags.BoolVar(&disableUploadTraffic, "disable-upload-traffic", false, "Disable traffic upload to panel")
	flags.BoolVar(&disableGetRule, "disable-get-rule", false, "Disable rule fetch from panel")

	return command
}

// promptCertSection handles the interactive TLS certificate mode selection.
func promptCertSection(writer io.Writer, reader *bufio.Reader, certMode string) (string, error) {
	if certMode != "" {
		return certMode, nil
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
		return "dns", nil
	case "2":
		return "http", nil
	case "3":
		return "tls", nil
	case "4":
		return "file", nil
	case "5":
		return "none", nil
	default:
		return "dns", nil
	}
}

// promptDNSEnv collects DNS provider environment variables interactively.
func promptDNSEnv(writer io.Writer, reader *bufio.Reader) (string, error) {
	fmt.Fprintln(writer, "  DNS environment variables (KEY=VALUE, enter empty line to finish):")
	var pairs []string
	for {
		line, err := prompt(writer, reader, "    ")
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.Contains(line, "=") {
			pairs = append(pairs, line)
		}
	}
	return strings.Join(pairs, ","), nil
}

// parseDNSEnv parses "KEY1=VAL1,KEY2=VAL2" into a map.
func parseDNSEnv(raw string) map[string]string {
	result := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// splitTrim splits a comma-separated string and trims each element.
func splitTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// promptRealitySection handles the interactive REALITY configuration.
func promptRealitySection(writer io.Writer, reader *bufio.Reader,
	enableReality, realityShow bool,
	dest, serverNames, privateKey, shortIds string,
	minClientVer, maxClientVer string, maxTimeDiff uint64,
) (bool, bool, string, string, string, string, string, string, uint64, error) {

	if enableReality {
		if dest == "" {
			dest = "www.amazon.com:443"
		}
		if serverNames == "" {
			serverNames = "www.amazon.com"
		}
		return true, realityShow, dest, serverNames, privateKey, shortIds, minClientVer, maxClientVer, maxTimeDiff, nil
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

	newDest, err := promptDefault(writer, reader, "  Destination", "www.amazon.com:443")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	newServerNames, err := promptDefault(writer, reader, "  Server names (comma-separated)", "www.amazon.com")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	newPrivateKey, err := promptDefault(writer, reader, "  Private key (leave empty to skip)", "")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	newShortIds, err := promptDefault(writer, reader, "  Short IDs (comma-separated, e.g. abc123,def456)", "")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	newMinVer, err := promptDefault(writer, reader, "  Min client version (optional)", "")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	newMaxVer, err := promptDefault(writer, reader, "  Max client version (optional)", "")
	if err != nil {
		return false, false, "", "", "", "", "", "", 0, err
	}

	return true, show, newDest, newServerNames, newPrivateKey, newShortIds, newMinVer, newMaxVer, 0, nil
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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/common/limiter"
	"github.com/XrayR-project/XrayR/common/mylego"
	xconfig "github.com/XrayR-project/XrayR/config"
	"github.com/XrayR-project/XrayR/internal/configui"
	"github.com/XrayR-project/XrayR/panel"
	"github.com/XrayR-project/XrayR/preflight"
	"github.com/XrayR-project/XrayR/service/controller"
)

func init() { configCmd.AddCommand(newConfigInitCommand()) }

type configInitOptions struct {
	panelName string
	apiHost   string
	apiKey    string
	nodeType  string
	output    string
	nodeID    int

	force      bool
	skipVerify bool
	skipDoctor bool

	enableVless bool
	vlessFlow   string

	certMode             string
	certDomain           string
	certProvider         string
	certEmail            string
	certFile             string
	certKeyFile          string
	certDNSEnv           string
	certRejectUnknownSNI bool

	enableREALITY             bool
	realityShow               bool
	realityDest               string
	realityServerNames        string
	realityPrivateKey         string
	realityShortIDs           string
	realityMinClientVer       string
	realityMaxClientVer       string
	realityMaxTimeDiff        uint64
	realityProxyProtocolVer   uint64
	disableLocalREALITYConfig bool

	listenIP    string
	sendIP      string
	speedLimit  float64
	deviceLimit int

	redisEnable   bool
	redisNetwork  string
	redisAddr     string
	redisUsername string
	redisPassword string
	redisDB       int
	redisTimeout  int
	redisExpiry   int

	enableProxyProtocol  bool
	enableFallback       bool
	fallbackSNI          string
	fallbackALPN         string
	fallbackPath         string
	fallbackDest         string
	fallbackProxyVer     uint64
	ruleListPath         string
	disableCustomConfig  bool
	disableUploadTraffic bool
	disableGetRule       bool
	disableIVCheck       bool
	disableSniffing      bool

	autoSpeedEnable   bool
	autoSpeedLimit    int
	autoSpeedWarn     int
	autoSpeedLimited  int
	autoSpeedDuration int

	updatePeriodic int
	enableDNS      bool
	dnsType        string
}

func newConfigInitCommand() *cobra.Command {
	options := &configInitOptions{realityShow: true, redisNetwork: "tcp", redisTimeout: 5, redisExpiry: 60, updatePeriodic: 60, dnsType: "AsIs"}
	command := &cobra.Command{
		Use:   "init",
		Short: "Create or edit configuration with a terminal UI or flags",
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.output == "" {
				options.output = resolveConfigPath()
			}
			if options.hasRequiredFlags() {
				return runConfigInitNonInteractive(cmd, options)
			}
			stdin, stdinOK := cmd.InOrStdin().(*os.File)
			stdout, stdoutOK := cmd.OutOrStdout().(*os.File)
			if !stdinOK || !stdoutOK || !term.IsTerminal(int(stdin.Fd())) || !term.IsTerminal(int(stdout.Fd())) {
				return errors.New("interactive config init requires a terminal; provide --panel, --api-host, --api-key, --node-id, and --node-type for non-interactive use")
			}
			var existing *panel.Config
			if loaded, err := xconfig.Load(options.output); err == nil {
				existing = loaded.Config
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("load existing config %s: %w", options.output, err)
			}
			cfg, err := configui.Run(existing, configui.Options{
				Output: options.output, Force: options.force, SkipVerify: options.skipVerify,
				SkipDoctor: options.skipDoctor, Input: stdin, Writer: stdout,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Configuration written to %s\n", options.output)
			fmt.Fprintf(cmd.OutOrStdout(), "Configured %d node(s). Next: systemctl enable --now XrayR\n", len(cfg.NodesConfig))
			return nil
		},
	}
	bindConfigInitFlags(command, options)
	return command
}

func (o *configInitOptions) hasRequiredFlags() bool {
	return strings.TrimSpace(o.panelName) != "" && strings.TrimSpace(o.apiHost) != "" && strings.TrimSpace(o.apiKey) != "" && o.nodeID > 0 && strings.TrimSpace(o.nodeType) != ""
}

func runConfigInitNonInteractive(cmd *cobra.Command, o *configInitOptions) error {
	definition, err := panel.LookupPanel(o.panelName)
	if err != nil {
		return err
	}
	nodeType := canonicalNodeType(definition, o.nodeType)
	if nodeType == "" {
		return fmt.Errorf("%s does not support node type %s", definition.Name, o.nodeType)
	}
	apiCfg := &api.Config{
		APIHost: o.apiHost, Key: o.apiKey, NodeID: o.nodeID, NodeType: nodeType,
		Timeout: 30, EnableVless: o.enableVless, VlessFlow: o.vlessFlow,
		SpeedLimit: o.speedLimit, DeviceLimit: o.deviceLimit, RuleListPath: o.ruleListPath,
		DisableCustomConfig: o.disableCustomConfig,
	}
	ctrl := &controller.Config{
		ListenIP: o.listenIP, SendIP: o.sendIP, UpdatePeriodic: o.updatePeriodic,
		EnableDNS: o.enableDNS, DNSType: o.dnsType,
		DisableUploadTraffic: o.disableUploadTraffic, DisableGetRule: o.disableGetRule,
		EnableProxyProtocol: o.enableProxyProtocol, DisableIVCheck: o.disableIVCheck,
		DisableSniffing: o.disableSniffing,
	}
	if o.certMode != "" && o.certMode != "none" {
		ctrl.CertConfig = &mylego.CertConfig{
			CertMode: o.certMode, CertDomain: o.certDomain, CertFile: o.certFile,
			KeyFile: o.certKeyFile, Provider: o.certProvider, Email: o.certEmail,
			DNSEnv: parseKeyValuePairs(o.certDNSEnv), RejectUnknownSni: o.certRejectUnknownSNI,
		}
	}
	if o.enableREALITY {
		ctrl.EnableREALITY = true
		ctrl.DisableLocalREALITYConfig = o.disableLocalREALITYConfig
		ctrl.REALITYConfigs = &controller.REALITYConfig{
			Show: o.realityShow, Dest: o.realityDest, ProxyProtocolVer: o.realityProxyProtocolVer,
			ServerNames: splitValues(o.realityServerNames), PrivateKey: o.realityPrivateKey,
			MinClientVer: o.realityMinClientVer, MaxClientVer: o.realityMaxClientVer,
			MaxTimeDiff: o.realityMaxTimeDiff, ShortIds: splitValues(o.realityShortIDs),
		}
		if strings.TrimSpace(o.realityShortIDs) == "" {
			ctrl.REALITYConfigs.ShortIds = []string{""}
		}
	}
	if o.enableFallback {
		ctrl.EnableFallback = true
		ctrl.FallBackConfigs = []*controller.FallBackConfig{{
			SNI: o.fallbackSNI, Alpn: o.fallbackALPN, Path: o.fallbackPath,
			Dest: o.fallbackDest, ProxyProtocolVer: o.fallbackProxyVer,
		}}
	}
	if o.redisEnable {
		ctrl.GlobalDeviceLimitConfig = &limiter.GlobalDeviceLimitConfig{
			Enable: true, RedisNetwork: o.redisNetwork, RedisAddr: o.redisAddr,
			RedisUsername: o.redisUsername, RedisPassword: o.redisPassword, RedisDB: o.redisDB,
			Timeout: o.redisTimeout, Expiry: o.redisExpiry,
		}
	}
	if o.autoSpeedEnable {
		ctrl.AutoSpeedLimitConfig = &controller.AutoSpeedLimitConfig{
			Limit: o.autoSpeedLimit, WarnTimes: o.autoSpeedWarn,
			LimitSpeed: o.autoSpeedLimited, LimitDuration: o.autoSpeedDuration,
		}
	}

	cfg := &panel.Config{
		ConfigVersion: xconfig.CurrentVersion,
		LogConfig:     &panel.LogConfig{Level: "info", Format: "text"},
		NodesConfig: []*panel.NodesConfig{{
			PanelType: definition.Name, ApiConfig: apiCfg, ControllerConfig: ctrl,
		}},
	}
	xconfig.ApplyDefaults(cfg, filepath.Dir(o.output))
	issues := xconfig.Validate(cfg)
	for _, issue := range issues {
		if issue.Severity == xconfig.SeverityError {
			return fmt.Errorf("%s: %s", issue.Path, issue.Message)
		}
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := xconfig.WriteAtomic(o.output, data, o.force); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output %s exists; use --force to replace it", o.output)
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Configuration written to %s\n", o.output)
	fmt.Fprintln(cmd.OutOrStdout(), "✓ Local validation passed")
	if !o.skipDoctor {
		results := preflight.Run(context.Background(), cfg, preflight.Options{Node: -1, Timeout: 5 * time.Second, Remote: !o.skipVerify})
		failed := false
		for _, result := range results {
			if result.Status == preflight.StatusError {
				failed = true
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s\n", result.Status, result.Name, result.Detail)
		}
		if failed {
			return errors.New("configuration was written, but verification failed; run XrayR doctor for details")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "✓ Doctor checks passed")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Next: systemctl enable --now XrayR (config: %s)\n", o.output)
	return nil
}

func bindConfigInitFlags(command *cobra.Command, o *configInitOptions) {
	flags := command.Flags()
	flags.StringVar(&o.panelName, "panel", "", "Panel type")
	flags.StringVar(&o.apiHost, "api-host", "", "Panel URL")
	flags.StringVar(&o.apiKey, "api-key", "", "Panel API key")
	flags.IntVar(&o.nodeID, "node-id", 0, "Node ID")
	flags.StringVar(&o.nodeType, "node-type", "", "Node protocol")
	flags.StringVarP(&o.output, "output", "o", "", "Output path")
	flags.BoolVar(&o.force, "force", false, "Overwrite output")
	flags.BoolVar(&o.skipVerify, "skip-verify", false, "Skip remote verification")
	flags.BoolVar(&o.skipDoctor, "skip-doctor", false, "Skip all Doctor checks")
	flags.BoolVar(&o.enableVless, "enable-vless", false, "Enable VLESS for V2ray nodes")
	flags.StringVar(&o.vlessFlow, "vless-flow", "", "VLESS flow")
	flags.StringVar(&o.certMode, "cert-mode", "", "TLS certificate mode (none/file/http/tls/dns)")
	flags.StringVar(&o.certDomain, "cert-domain", "", "Certificate domain")
	flags.StringVar(&o.certProvider, "cert-provider", "", "DNS provider for ACME challenge")
	flags.StringVar(&o.certEmail, "cert-email", "", "Email for ACME")
	flags.StringVar(&o.certFile, "cert-file", "", "TLS certificate file")
	flags.StringVar(&o.certKeyFile, "cert-key-file", "", "TLS private key file")
	flags.StringVar(&o.certDNSEnv, "cert-dns-env", "", "DNS environment (KEY=VALUE,KEY2=VALUE2)")
	flags.BoolVar(&o.certRejectUnknownSNI, "cert-reject-unknown-sni", false, "Reject unknown SNI")
	flags.BoolVar(&o.enableREALITY, "enable-reality", false, "Enable REALITY")
	flags.BoolVar(&o.realityShow, "reality-show", true, "Show REALITY debug information")
	flags.StringVar(&o.realityDest, "reality-dest", "www.amazon.com:443", "REALITY destination")
	flags.StringVar(&o.realityServerNames, "reality-server-names", "www.amazon.com", "REALITY server names")
	flags.StringVar(&o.realityPrivateKey, "reality-private-key", "", "REALITY private key")
	flags.StringVar(&o.realityShortIDs, "reality-short-ids", "", "REALITY short IDs")
	flags.StringVar(&o.realityMinClientVer, "reality-min-client-ver", "", "REALITY minimum client version")
	flags.StringVar(&o.realityMaxClientVer, "reality-max-client-ver", "", "REALITY maximum client version")
	flags.Uint64Var(&o.realityMaxTimeDiff, "reality-max-time-diff", 0, "REALITY maximum time difference")
	flags.Uint64Var(&o.realityProxyProtocolVer, "reality-proxy-protocol-ver", 0, "REALITY PROXY protocol version")
	flags.BoolVar(&o.disableLocalREALITYConfig, "disable-local-reality-config", false, "Use panel REALITY config only")
	flags.StringVar(&o.listenIP, "listen-ip", "", "Listen IP")
	flags.StringVar(&o.sendIP, "send-ip", "", "Outbound IP")
	flags.Float64Var(&o.speedLimit, "speed-limit", 0, "Speed limit in Mbps")
	flags.IntVar(&o.deviceLimit, "device-limit", 0, "Device limit")
	flags.BoolVar(&o.redisEnable, "redis-enable", false, "Enable Redis global device limit")
	flags.StringVar(&o.redisNetwork, "redis-network", "tcp", "Redis network (tcp/unix)")
	flags.StringVar(&o.redisAddr, "redis-addr", "", "Redis address")
	flags.StringVar(&o.redisUsername, "redis-username", "", "Redis username")
	flags.StringVar(&o.redisPassword, "redis-password", "", "Redis password")
	flags.IntVar(&o.redisDB, "redis-db", 0, "Redis DB")
	flags.IntVar(&o.redisTimeout, "redis-timeout", 5, "Redis timeout")
	flags.IntVar(&o.redisExpiry, "redis-expiry", 60, "Redis expiry")
	flags.BoolVar(&o.enableProxyProtocol, "enable-proxy-protocol", false, "Enable PROXY protocol")
	flags.BoolVar(&o.enableFallback, "enable-fallback", false, "Enable fallback")
	flags.StringVar(&o.fallbackDest, "fallback-dest", "", "Fallback destination")
	flags.StringVar(&o.fallbackSNI, "fallback-sni", "", "Fallback SNI")
	flags.StringVar(&o.fallbackALPN, "fallback-alpn", "", "Fallback ALPN")
	flags.StringVar(&o.fallbackPath, "fallback-path", "", "Fallback HTTP path")
	flags.Uint64Var(&o.fallbackProxyVer, "fallback-proxy-protocol-ver", 0, "Fallback PROXY protocol version")
	flags.StringVar(&o.ruleListPath, "rule-list-path", "", "Rule list path")
	flags.BoolVar(&o.disableCustomConfig, "disable-custom-config", false, "Disable panel custom config")
	flags.BoolVar(&o.disableUploadTraffic, "disable-upload-traffic", false, "Disable traffic upload")
	flags.BoolVar(&o.disableGetRule, "disable-get-rule", false, "Disable rule synchronization")
	flags.BoolVar(&o.disableIVCheck, "disable-iv-check", false, "Disable Shadowsocks IV check")
	flags.BoolVar(&o.disableSniffing, "disable-sniffing", false, "Disable sniffing")
	flags.BoolVar(&o.autoSpeedEnable, "auto-speed-enable", false, "Enable automatic speed limiting")
	flags.IntVar(&o.autoSpeedLimit, "auto-speed-limit", 0, "Automatic speed trigger")
	flags.IntVar(&o.autoSpeedWarn, "auto-speed-warn-times", 0, "Warnings before limiting")
	flags.IntVar(&o.autoSpeedLimited, "auto-speed-limited-speed", 0, "Limited speed")
	flags.IntVar(&o.autoSpeedDuration, "auto-speed-duration", 0, "Limit duration")
	flags.IntVar(&o.updatePeriodic, "update-periodic", 60, "Panel sync interval")
	flags.BoolVar(&o.enableDNS, "enable-dns", false, "Enable custom DNS")
	flags.StringVar(&o.dnsType, "dns-type", "AsIs", "DNS strategy")
}

func canonicalNodeType(definition panel.PanelDefinition, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	for _, supported := range definition.NodeTypes {
		if strings.EqualFold(supported, candidate) {
			return supported
		}
	}
	return ""
}

func parseKeyValuePairs(raw string) map[string]string {
	result := map[string]string{}
	for _, pair := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' }) {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func splitValues(raw string) []string {
	var result []string
	for _, value := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' }) {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

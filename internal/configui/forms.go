package configui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
)

func globalGroups(state *State) []*huh.Group {
	g := &state.Global
	return []*huh.Group{
		huh.NewGroup(
			huh.NewNote().Title("XrayR Configuration").Description("Global logging, connection, diagnostics and cache settings."),
			huh.NewSelect[string]().Title("Log level").Options(huh.NewOptions("none", "error", "warning", "info", "debug")...).Value(&g.LogLevel),
			huh.NewSelect[string]().Title("Log format").Options(huh.NewOptions("text", "json")...).Value(&g.LogFormat),
			huh.NewInput().Title("Access log path").Description("Leave empty to disable file logging.").Value(&g.LogAccessPath),
			huh.NewInput().Title("Error log path").Description("Leave empty to disable file logging.").Value(&g.LogErrorPath),
		),
		huh.NewGroup(
			huh.NewInput().Title("Handshake timeout (seconds)").Value(&g.Handshake).Validate(validatePositiveInt),
			huh.NewInput().Title("Connection idle timeout (seconds)").Value(&g.ConnIdle).Validate(validatePositiveInt),
			huh.NewInput().Title("Uplink-only timeout (seconds)").Value(&g.UplinkOnly).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Downlink-only timeout (seconds)").Value(&g.DownlinkOnly).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Connection buffer size (KB)").Value(&g.BufferSize).Validate(validateNonNegativeInt),
		),
		huh.NewGroup(
			huh.NewInput().Title("DNS config JSON path").Value(&g.DNSConfigPath),
			huh.NewInput().Title("Routing config JSON path").Value(&g.RouteConfigPath),
			huh.NewInput().Title("Custom inbound JSON path").Value(&g.InboundConfigPath),
			huh.NewInput().Title("Custom outbound JSON path").Value(&g.OutboundConfigPath),
		),
		huh.NewGroup(
			huh.NewConfirm().Title("Enable diagnostics HTTP server?").Value(&g.DiagnosticsEnable),
		),
		huh.NewGroup(
			huh.NewInput().Title("Diagnostics listen address").Value(&g.DiagnosticsListen).Validate(validateListen),
		).WithHideFunc(func() bool { return !g.DiagnosticsEnable }),
		huh.NewGroup(
			huh.NewConfirm().Title("Enable last-known-good snapshot cache?").Value(&g.CacheEnable),
		),
		huh.NewGroup(
			huh.NewInput().Title("Cache directory").Value(&g.CachePath).Validate(validateRequired),
			huh.NewInput().Title("Cache maximum age (seconds)").Value(&g.CacheMaxAge).Validate(validatePositiveInt),
		).WithHideFunc(func() bool { return !g.CacheEnable }),
	}
}

func nodeGroups(node *NodeState) []*huh.Group {
	groups := []*huh.Group{
		huh.NewGroup(
			huh.NewNote().Title("Panel and node").Description("Select the panel adapter and identify the remote node."),
			huh.NewSelect[string]().Title("Panel type").Options(panelOptions()...).Value(&node.PanelType),
			huh.NewSelect[string]().Title("Node type").OptionsFunc(func() []huh.Option[string] { return nodeTypeOptions(node.PanelType) }, &node.PanelType).Value(&node.NodeType),
			huh.NewInput().Title("Panel URL").Value(&node.APIHost).Validate(validateURL),
			huh.NewInput().Title("API key").Value(&node.APIKey).EchoMode(huh.EchoModePassword).Validate(validateRequired),
			huh.NewInput().Title("Node ID").Value(&node.NodeID).Validate(validatePositiveInt),
			huh.NewInput().Title("Panel request timeout (seconds)").Value(&node.Timeout).Validate(validatePositiveInt),
		),
		huh.NewGroup(
			huh.NewConfirm().Title("Enable VLESS for V2ray node?").Value(&node.EnableVless),
		).WithHideFunc(func() bool { return !strings.EqualFold(node.NodeType, "V2ray") }),
		huh.NewGroup(
			huh.NewInput().Title("VLESS flow").Value(&node.VlessFlow),
		).WithHideFunc(func() bool { return !isVLESS(node.NodeType, node.EnableVless) }),
		huh.NewGroup(
			huh.NewInput().Title("Local speed limit (Mbps)").Value(&node.SpeedLimit).Validate(validateNonNegativeFloat),
			huh.NewInput().Title("Local device limit").Value(&node.DeviceLimit).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Local rule list path").Value(&node.RuleListPath),
			huh.NewConfirm().Title("Disable panel custom configuration?").Value(&node.DisableCustomConfig),
		),
		huh.NewGroup(
			huh.NewNote().Title("Controller").Description("Network binding, synchronization, DNS and reporting behavior."),
			huh.NewInput().Title("Listen IP").Value(&node.ListenIP).Validate(validateRequired),
			huh.NewInput().Title("Outbound send IP").Value(&node.SendIP).Validate(validateRequired),
			huh.NewInput().Title("Update interval (seconds)").Value(&node.UpdatePeriodic).Validate(validatePositiveInt),
			huh.NewConfirm().Title("Enable custom DNS?").Value(&node.EnableDNS),
			huh.NewSelect[string]().Title("DNS strategy").Options(huh.NewOptions("AsIs", "UseIP", "UseIPv4", "UseIPv6")...).Value(&node.DNSType),
			huh.NewConfirm().Title("Enable PROXY protocol?").Value(&node.EnableProxyProtocol),
			huh.NewConfirm().Title("Disable traffic upload?").Value(&node.DisableUploadTraffic),
			huh.NewConfirm().Title("Disable rule synchronization?").Value(&node.DisableGetRule),
			huh.NewConfirm().Title("Disable Shadowsocks IV check?").Value(&node.DisableIVCheck),
			huh.NewConfirm().Title("Disable traffic sniffing?").Value(&node.DisableSniffing),
		),
	}
	groups = append(groups, certGroups(node)...)
	groups = append(groups, realityGroups(node)...)
	groups = append(groups, limitGroups(node)...)
	groups = append(groups, fallbackGroups(node)...)
	return groups
}

func certGroups(node *NodeState) []*huh.Group {
	return []*huh.Group{
		huh.NewGroup(
			huh.NewNote().Title("TLS certificate").Description("REALITY and local TLS can be configured independently; runtime behavior selects REALITY when enabled."),
			huh.NewSelect[string]().Title("Certificate mode").Options(
				huh.NewOption("None", "none"), huh.NewOption("Existing files", "file"),
				huh.NewOption("HTTP-01", "http"), huh.NewOption("TLS-ALPN-01", "tls"), huh.NewOption("DNS-01", "dns"),
			).Value(&node.CertMode),
		),
		huh.NewGroup(
			huh.NewInput().Title("Certificate file").Value(&node.CertFile).Validate(validateRequired),
			huh.NewInput().Title("Private key file").Value(&node.CertKeyFile).Validate(validateRequired),
			huh.NewConfirm().Title("Reject unknown SNI?").Value(&node.CertRejectUnknownSNI),
		).WithHideFunc(func() bool { return node.CertMode != "file" }),
		huh.NewGroup(
			huh.NewInput().Title("Certificate domain").Value(&node.CertDomain).Validate(validateRequired),
			huh.NewInput().Title("ACME account email").Value(&node.CertEmail).Validate(validateRequired),
			huh.NewConfirm().Title("Reject unknown SNI?").Value(&node.CertRejectUnknownSNI),
		).WithHideFunc(func() bool { return !acmeMode(node.CertMode) }),
		huh.NewGroup(
			huh.NewInput().Title("DNS provider").Description("Provider name used by lego, for example alidns or cloudflare.").Value(&node.CertProvider).Validate(validateRequired),
			huh.NewInput().Title("DNS provider environment").Description("Comma-separated KEY=VALUE pairs. Input and review output are masked.").Value(&node.CertDNSEnv).EchoMode(huh.EchoModePassword).Validate(validateEnv),
		).WithHideFunc(func() bool { return node.CertMode != "dns" }),
	}
}

func realityGroups(node *NodeState) []*huh.Group {
	return []*huh.Group{
		huh.NewGroup(
			huh.NewNote().Title("REALITY"),
			huh.NewConfirm().Title("Enable REALITY?").Value(&node.EnableREALITY),
		),
		huh.NewGroup(
			huh.NewConfirm().Title("Use panel-supplied REALITY config only?").Value(&node.DisableLocalREALITYConfig),
		).WithHideFunc(func() bool { return !node.EnableREALITY }),
		huh.NewGroup(
			huh.NewConfirm().Title("Show REALITY debug information?").Value(&node.RealityShow),
			huh.NewInput().Title("Destination").Value(&node.RealityDest).Validate(validateRequired),
			huh.NewText().Title("Server names").Description("One hostname per line.").Lines(4).Value(&node.RealityServerNames).Validate(validateRequired),
			huh.NewInput().Title("Private key").Value(&node.RealityPrivateKey).EchoMode(huh.EchoModePassword).Validate(validateRequired),
			huh.NewText().Title("Short IDs").Description("One value per line; an empty list writes a single empty short ID.").Lines(4).Value(&node.RealityShortIDs),
			huh.NewInput().Title("Minimum client version").Value(&node.RealityMinClientVer),
			huh.NewInput().Title("Maximum client version").Value(&node.RealityMaxClientVer),
			huh.NewInput().Title("Maximum time difference (ms)").Value(&node.RealityMaxTimeDiff).Validate(validateNonNegativeInt),
			huh.NewInput().Title("PROXY protocol version").Value(&node.RealityProxyProtocolVer).Validate(validateNonNegativeInt),
		).WithHideFunc(func() bool { return !node.EnableREALITY || node.DisableLocalREALITYConfig }),
	}
}

func limitGroups(node *NodeState) []*huh.Group {
	return []*huh.Group{
		huh.NewGroup(
			huh.NewNote().Title("Automatic speed limiting"),
			huh.NewConfirm().Title("Enable automatic speed limiting?").Value(&node.AutoSpeedLimitEnable),
		),
		huh.NewGroup(
			huh.NewInput().Title("Trigger speed (Mbps)").Value(&node.AutoSpeedLimit).Validate(validatePositiveInt),
			huh.NewInput().Title("Warnings before limiting").Value(&node.AutoSpeedWarnTimes).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Limited speed (Mbps)").Value(&node.AutoSpeedLimitSpeed).Validate(validatePositiveInt),
			huh.NewInput().Title("Limit duration (minutes)").Value(&node.AutoSpeedLimitDuration).Validate(validatePositiveInt),
		).WithHideFunc(func() bool { return !node.AutoSpeedLimitEnable }),
		huh.NewGroup(
			huh.NewNote().Title("Global device limiting"),
			huh.NewConfirm().Title("Enable Redis global device limiting?").Value(&node.RedisEnable),
		),
		huh.NewGroup(
			huh.NewSelect[string]().Title("Redis network").Options(huh.NewOptions("tcp", "unix")...).Value(&node.RedisNetwork),
			huh.NewInput().Title("Redis address or socket path").Value(&node.RedisAddr).Validate(validateRequired),
			huh.NewInput().Title("Redis username").Value(&node.RedisUsername),
			huh.NewInput().Title("Redis password").Value(&node.RedisPassword).EchoMode(huh.EchoModePassword),
			huh.NewInput().Title("Redis DB").Value(&node.RedisDB).Validate(validateNonNegativeInt),
			huh.NewInput().Title("Redis timeout (seconds)").Value(&node.RedisTimeout).Validate(validatePositiveInt),
			huh.NewInput().Title("Redis entry expiry (seconds)").Value(&node.RedisExpiry).Validate(validatePositiveInt),
		).WithHideFunc(func() bool { return !node.RedisEnable }),
	}
}

func fallbackGroups(node *NodeState) []*huh.Group {
	groups := []*huh.Group{
		huh.NewGroup(
			huh.NewNote().Title("Fallbacks").Description("Fallbacks are available only for Trojan and VLESS nodes."),
			huh.NewConfirm().Title("Enable fallback routing?").Value(&node.EnableFallback),
		).WithHideFunc(func() bool { return !supportsFallback(node.NodeType, node.EnableVless) }),
	}
	for index := range node.Fallbacks {
		fallback := &node.Fallbacks[index]
		groups = append(groups, huh.NewGroup(
			huh.NewNote().Title(fmt.Sprintf("Fallback %d", index+1)),
			huh.NewInput().Title("SNI").Value(&fallback.SNI),
			huh.NewInput().Title("ALPN").Value(&fallback.ALPN),
			huh.NewInput().Title("HTTP path").Value(&fallback.Path),
			huh.NewInput().Title("Destination").Value(&fallback.Dest).Validate(validateRequired),
			huh.NewInput().Title("PROXY protocol version").Value(&fallback.ProxyProtocolVer).Validate(validateNonNegativeInt),
		).WithHideFunc(func() bool { return !supportsFallback(node.NodeType, node.EnableVless) || !node.EnableFallback }))
	}
	return groups
}

func nodeSummary(node NodeState, index int) string {
	id := node.NodeID
	if strings.TrimSpace(id) == "" {
		id = "?"
	}
	host := node.APIHost
	if strings.TrimSpace(host) == "" {
		host = "not configured"
	}
	return fmt.Sprintf("%d. %s / %s / node %s / %s", index+1, node.PanelType, node.NodeType, id, host)
}

func integerDescription(value string) string {
	if _, err := strconv.Atoi(value); err != nil {
		return "Invalid integer"
	}
	return ""
}

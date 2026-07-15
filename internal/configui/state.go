package configui

import (
	"strconv"
	"strings"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/common/limiter"
	"github.com/XrayR-project/XrayR/common/mylego"
	xconfig "github.com/XrayR-project/XrayR/config"
	"github.com/XrayR-project/XrayR/panel"
	"github.com/XrayR-project/XrayR/service/controller"
)

// State is the mutable backing model used by the terminal configuration UI.
type State struct {
	Global GlobalState
	Nodes  []NodeState
}

type GlobalState struct {
	LogLevel           string
	LogFormat          string
	LogAccessPath      string
	LogErrorPath       string
	DNSConfigPath      string
	InboundConfigPath  string
	OutboundConfigPath string
	RouteConfigPath    string

	Handshake    string
	ConnIdle     string
	UplinkOnly   string
	DownlinkOnly string
	BufferSize   string

	DiagnosticsEnable bool
	DiagnosticsListen string
	CacheEnable       bool
	CachePath         string
	CacheMaxAge       string
}

type NodeState struct {
	PanelType string
	APIHost   string
	APIKey    string
	NodeID    string
	NodeType  string
	Timeout   string

	EnableVless         bool
	VlessFlow           string
	SpeedLimit          string
	DeviceLimit         string
	RuleListPath        string
	DisableCustomConfig bool

	ListenIP             string
	SendIP               string
	UpdatePeriodic       string
	EnableDNS            bool
	DNSType              string
	DisableUploadTraffic bool
	DisableGetRule       bool
	EnableProxyProtocol  bool
	DisableIVCheck       bool
	DisableSniffing      bool

	CertMode             string
	CertDomain           string
	CertFile             string
	CertKeyFile          string
	CertProvider         string
	CertEmail            string
	CertDNSEnv           string
	CertRejectUnknownSNI bool

	EnableREALITY             bool
	DisableLocalREALITYConfig bool
	RealityShow               bool
	RealityDest               string
	RealityProxyProtocolVer   string
	RealityServerNames        string
	RealityPrivateKey         string
	RealityMinClientVer       string
	RealityMaxClientVer       string
	RealityMaxTimeDiff        string
	RealityShortIDs           string

	AutoSpeedLimitEnable   bool
	AutoSpeedLimit         string
	AutoSpeedWarnTimes     string
	AutoSpeedLimitSpeed    string
	AutoSpeedLimitDuration string

	RedisEnable   bool
	RedisNetwork  string
	RedisAddr     string
	RedisUsername string
	RedisPassword string
	RedisDB       string
	RedisTimeout  string
	RedisExpiry   string

	EnableFallback bool
	Fallbacks      []FallbackState
}

type FallbackState struct {
	SNI              string
	ALPN             string
	Path             string
	Dest             string
	ProxyProtocolVer string
}

func NewState(configDir string) *State {
	cfg := &panel.Config{ConfigVersion: xconfig.CurrentVersion}
	xconfig.ApplyDefaults(cfg, configDir)
	return StateFromConfig(cfg, configDir)
}

func StateFromConfig(cfg *panel.Config, configDir string) *State {
	if cfg == nil {
		return NewState(configDir)
	}
	copyCfg := cloneConfig(cfg)
	xconfig.ApplyDefaults(copyCfg, configDir)

	state := &State{Global: globalFromConfig(copyCfg)}
	state.Nodes = make([]NodeState, 0, len(copyCfg.NodesConfig))
	for _, node := range copyCfg.NodesConfig {
		state.Nodes = append(state.Nodes, nodeFromConfig(node))
	}
	if len(state.Nodes) == 0 {
		state.Nodes = append(state.Nodes, defaultNodeState())
	}
	return state
}

func (s *State) Config(configDir string) (*panel.Config, error) {
	cfg := &panel.Config{
		ConfigVersion: xconfig.CurrentVersion,
		LogConfig: &panel.LogConfig{
			Level: s.Global.LogLevel, Format: s.Global.LogFormat,
			AccessPath: s.Global.LogAccessPath, ErrorPath: s.Global.LogErrorPath,
		},
		DnsConfigPath:      s.Global.DNSConfigPath,
		InboundConfigPath:  s.Global.InboundConfigPath,
		OutboundConfigPath: s.Global.OutboundConfigPath,
		RouteConfigPath:    s.Global.RouteConfigPath,
		ConnectionConfig: &panel.ConnectionConfig{
			Handshake: uint32(parseInt(s.Global.Handshake)), ConnIdle: uint32(parseInt(s.Global.ConnIdle)),
			UplinkOnly: uint32(parseInt(s.Global.UplinkOnly)), DownlinkOnly: uint32(parseInt(s.Global.DownlinkOnly)),
			BufferSize: int32(parseInt(s.Global.BufferSize)),
		},
		Diagnostics: &panel.DiagnosticsConfig{Enable: s.Global.DiagnosticsEnable, Listen: s.Global.DiagnosticsListen},
		Cache:       &panel.CacheConfig{Enable: s.Global.CacheEnable, Path: s.Global.CachePath, MaxAge: parseInt(s.Global.CacheMaxAge)},
	}
	for i := range s.Nodes {
		node, err := s.Nodes[i].Config()
		if err != nil {
			return nil, err
		}
		cfg.NodesConfig = append(cfg.NodesConfig, node)
	}
	xconfig.ApplyDefaults(cfg, configDir)
	return cfg, nil
}

func (s *State) AddNode() int {
	s.Nodes = append(s.Nodes, defaultNodeState())
	return len(s.Nodes) - 1
}

func (s *State) CloneNode(index int) int {
	if index < 0 || index >= len(s.Nodes) {
		return -1
	}
	clone := s.Nodes[index]
	clone.Fallbacks = append([]FallbackState(nil), s.Nodes[index].Fallbacks...)
	clone.NodeID = ""
	s.Nodes = append(s.Nodes, clone)
	return len(s.Nodes) - 1
}

func (s *State) RemoveNode(index int) bool {
	if index < 0 || index >= len(s.Nodes) || len(s.Nodes) == 1 {
		return false
	}
	s.Nodes = append(s.Nodes[:index], s.Nodes[index+1:]...)
	return true
}

func (n *NodeState) Config() (*panel.NodesConfig, error) {
	apiCfg := &api.Config{
		APIHost: n.APIHost, Key: n.APIKey, NodeID: parseInt(n.NodeID), NodeType: n.NodeType,
		Timeout: parseInt(n.Timeout), EnableVless: n.EnableVless, VlessFlow: n.VlessFlow,
		SpeedLimit: parseFloat(n.SpeedLimit), DeviceLimit: parseInt(n.DeviceLimit),
		RuleListPath: n.RuleListPath, DisableCustomConfig: n.DisableCustomConfig,
	}
	ctrl := &controller.Config{
		ListenIP: n.ListenIP, SendIP: n.SendIP, UpdatePeriodic: parseInt(n.UpdatePeriodic),
		EnableDNS: n.EnableDNS, DNSType: n.DNSType,
		DisableUploadTraffic: n.DisableUploadTraffic, DisableGetRule: n.DisableGetRule,
		EnableProxyProtocol: n.EnableProxyProtocol, DisableIVCheck: n.DisableIVCheck,
		DisableSniffing: n.DisableSniffing,
	}
	if n.CertMode != "" && n.CertMode != "none" {
		ctrl.CertConfig = &mylego.CertConfig{
			CertMode: n.CertMode, CertDomain: n.CertDomain, CertFile: n.CertFile,
			KeyFile: n.CertKeyFile, Provider: n.CertProvider, Email: n.CertEmail,
			DNSEnv: parseEnv(n.CertDNSEnv), RejectUnknownSni: n.CertRejectUnknownSNI,
		}
	}
	if n.EnableREALITY {
		ctrl.EnableREALITY = true
		ctrl.DisableLocalREALITYConfig = n.DisableLocalREALITYConfig
		ctrl.REALITYConfigs = &controller.REALITYConfig{
			Show: n.RealityShow, Dest: n.RealityDest,
			ProxyProtocolVer: parseUint(n.RealityProxyProtocolVer),
			ServerNames:      splitLines(n.RealityServerNames), PrivateKey: n.RealityPrivateKey,
			MinClientVer: n.RealityMinClientVer, MaxClientVer: n.RealityMaxClientVer,
			MaxTimeDiff: parseUint(n.RealityMaxTimeDiff), ShortIds: splitLinesKeepEmpty(n.RealityShortIDs),
		}
	}
	if n.AutoSpeedLimitEnable {
		ctrl.AutoSpeedLimitConfig = &controller.AutoSpeedLimitConfig{
			Limit: parseInt(n.AutoSpeedLimit), WarnTimes: parseInt(n.AutoSpeedWarnTimes),
			LimitSpeed: parseInt(n.AutoSpeedLimitSpeed), LimitDuration: parseInt(n.AutoSpeedLimitDuration),
		}
	}
	if n.RedisEnable {
		ctrl.GlobalDeviceLimitConfig = &limiter.GlobalDeviceLimitConfig{
			Enable: true, RedisNetwork: n.RedisNetwork, RedisAddr: n.RedisAddr,
			RedisUsername: n.RedisUsername, RedisPassword: n.RedisPassword, RedisDB: parseInt(n.RedisDB),
			Timeout: parseInt(n.RedisTimeout), Expiry: parseInt(n.RedisExpiry),
		}
	}
	if n.EnableFallback {
		ctrl.EnableFallback = true
		for _, fallback := range n.Fallbacks {
			ctrl.FallBackConfigs = append(ctrl.FallBackConfigs, &controller.FallBackConfig{
				SNI: fallback.SNI, Alpn: fallback.ALPN, Path: fallback.Path,
				Dest: fallback.Dest, ProxyProtocolVer: parseUint(fallback.ProxyProtocolVer),
			})
		}
	}
	return &panel.NodesConfig{PanelType: n.PanelType, ApiConfig: apiCfg, ControllerConfig: ctrl}, nil
}

func globalFromConfig(cfg *panel.Config) GlobalState {
	g := GlobalState{
		LogLevel: cfg.LogConfig.Level, LogFormat: cfg.LogConfig.Format,
		LogAccessPath: cfg.LogConfig.AccessPath, LogErrorPath: cfg.LogConfig.ErrorPath,
		DNSConfigPath: cfg.DnsConfigPath, InboundConfigPath: cfg.InboundConfigPath,
		OutboundConfigPath: cfg.OutboundConfigPath, RouteConfigPath: cfg.RouteConfigPath,
		Handshake:         strconv.FormatUint(uint64(cfg.ConnectionConfig.Handshake), 10),
		ConnIdle:          strconv.FormatUint(uint64(cfg.ConnectionConfig.ConnIdle), 10),
		UplinkOnly:        strconv.FormatUint(uint64(cfg.ConnectionConfig.UplinkOnly), 10),
		DownlinkOnly:      strconv.FormatUint(uint64(cfg.ConnectionConfig.DownlinkOnly), 10),
		BufferSize:        strconv.FormatInt(int64(cfg.ConnectionConfig.BufferSize), 10),
		DiagnosticsEnable: cfg.Diagnostics.Enable, DiagnosticsListen: cfg.Diagnostics.Listen,
		CacheEnable: cfg.Cache.Enable, CachePath: cfg.Cache.Path, CacheMaxAge: strconv.Itoa(cfg.Cache.MaxAge),
	}
	return g
}

func nodeFromConfig(node *panel.NodesConfig) NodeState {
	state := defaultNodeState()
	if node == nil {
		return state
	}
	state.PanelType = node.PanelType
	if node.ApiConfig != nil {
		a := node.ApiConfig
		state.APIHost, state.APIKey, state.NodeID, state.NodeType = a.APIHost, a.Key, strconv.Itoa(a.NodeID), a.NodeType
		state.Timeout, state.EnableVless, state.VlessFlow = strconv.Itoa(a.Timeout), a.EnableVless, a.VlessFlow
		state.SpeedLimit, state.DeviceLimit = formatFloat(a.SpeedLimit), strconv.Itoa(a.DeviceLimit)
		state.RuleListPath, state.DisableCustomConfig = a.RuleListPath, a.DisableCustomConfig
	}
	if node.ControllerConfig == nil {
		return state
	}
	c := node.ControllerConfig
	state.ListenIP, state.SendIP, state.UpdatePeriodic = c.ListenIP, c.SendIP, strconv.Itoa(c.UpdatePeriodic)
	state.EnableDNS, state.DNSType = c.EnableDNS, c.DNSType
	state.DisableUploadTraffic, state.DisableGetRule = c.DisableUploadTraffic, c.DisableGetRule
	state.EnableProxyProtocol, state.DisableIVCheck, state.DisableSniffing = c.EnableProxyProtocol, c.DisableIVCheck, c.DisableSniffing
	if c.CertConfig != nil {
		cert := c.CertConfig
		state.CertMode, state.CertDomain, state.CertFile, state.CertKeyFile = cert.CertMode, cert.CertDomain, cert.CertFile, cert.KeyFile
		state.CertProvider, state.CertEmail, state.CertRejectUnknownSNI = cert.Provider, cert.Email, cert.RejectUnknownSni
		state.CertDNSEnv = formatEnv(cert.DNSEnv)
	}
	state.EnableREALITY, state.DisableLocalREALITYConfig = c.EnableREALITY, c.DisableLocalREALITYConfig
	if c.REALITYConfigs != nil {
		r := c.REALITYConfigs
		state.RealityShow, state.RealityDest = r.Show, r.Dest
		state.RealityProxyProtocolVer = strconv.FormatUint(r.ProxyProtocolVer, 10)
		state.RealityServerNames, state.RealityPrivateKey = strings.Join(r.ServerNames, "\n"), r.PrivateKey
		state.RealityMinClientVer, state.RealityMaxClientVer = r.MinClientVer, r.MaxClientVer
		state.RealityMaxTimeDiff = strconv.FormatUint(r.MaxTimeDiff, 10)
		state.RealityShortIDs = strings.Join(r.ShortIds, "\n")
	}
	if c.AutoSpeedLimitConfig != nil && c.AutoSpeedLimitConfig.Limit > 0 {
		a := c.AutoSpeedLimitConfig
		state.AutoSpeedLimitEnable = true
		state.AutoSpeedLimit, state.AutoSpeedWarnTimes = strconv.Itoa(a.Limit), strconv.Itoa(a.WarnTimes)
		state.AutoSpeedLimitSpeed, state.AutoSpeedLimitDuration = strconv.Itoa(a.LimitSpeed), strconv.Itoa(a.LimitDuration)
	}
	if c.GlobalDeviceLimitConfig != nil && c.GlobalDeviceLimitConfig.Enable {
		r := c.GlobalDeviceLimitConfig
		state.RedisEnable, state.RedisNetwork, state.RedisAddr = true, r.RedisNetwork, r.RedisAddr
		state.RedisUsername, state.RedisPassword, state.RedisDB = r.RedisUsername, r.RedisPassword, strconv.Itoa(r.RedisDB)
		state.RedisTimeout, state.RedisExpiry = strconv.Itoa(r.Timeout), strconv.Itoa(r.Expiry)
	}
	state.EnableFallback = c.EnableFallback && supportsFallback(state.NodeType, state.EnableVless)
	for _, f := range c.FallBackConfigs {
		if f == nil {
			continue
		}
		state.Fallbacks = append(state.Fallbacks, FallbackState{
			SNI: f.SNI, ALPN: f.Alpn, Path: f.Path, Dest: f.Dest,
			ProxyProtocolVer: strconv.FormatUint(f.ProxyProtocolVer, 10),
		})
	}
	return state
}

func defaultNodeState() NodeState {
	return NodeState{
		PanelType: "Xboard", NodeType: "Vless", Timeout: "30", VlessFlow: "xtls-rprx-vision",
		ListenIP: "0.0.0.0", SendIP: "0.0.0.0", UpdatePeriodic: "60", DNSType: "AsIs",
		CertMode: "none", RealityShow: true, RealityDest: "www.amazon.com:443",
		RealityProxyProtocolVer: "0", RealityServerNames: "www.amazon.com", RealityMaxTimeDiff: "0",
		AutoSpeedLimit: "0", AutoSpeedWarnTimes: "0", AutoSpeedLimitSpeed: "0", AutoSpeedLimitDuration: "0",
		RedisNetwork: "tcp", RedisAddr: "127.0.0.1:6379", RedisDB: "0", RedisTimeout: "5", RedisExpiry: "60",
		SpeedLimit: "0", DeviceLimit: "0", Fallbacks: []FallbackState{{Dest: "80", ProxyProtocolVer: "0"}},
	}
}

func cloneConfig(cfg *panel.Config) *panel.Config {
	state := &State{Global: globalFromConfigUnsafe(cfg)}
	for _, node := range cfg.NodesConfig {
		state.Nodes = append(state.Nodes, nodeFromConfig(node))
	}
	copyCfg, _ := state.Config(".")
	return copyCfg
}

func globalFromConfigUnsafe(cfg *panel.Config) GlobalState {
	g := GlobalState{}
	if cfg.LogConfig != nil {
		g.LogLevel, g.LogFormat = cfg.LogConfig.Level, cfg.LogConfig.Format
		g.LogAccessPath, g.LogErrorPath = cfg.LogConfig.AccessPath, cfg.LogConfig.ErrorPath
	}
	g.DNSConfigPath, g.InboundConfigPath = cfg.DnsConfigPath, cfg.InboundConfigPath
	g.OutboundConfigPath, g.RouteConfigPath = cfg.OutboundConfigPath, cfg.RouteConfigPath
	if cfg.ConnectionConfig != nil {
		g.Handshake = strconv.FormatUint(uint64(cfg.ConnectionConfig.Handshake), 10)
		g.ConnIdle = strconv.FormatUint(uint64(cfg.ConnectionConfig.ConnIdle), 10)
		g.UplinkOnly = strconv.FormatUint(uint64(cfg.ConnectionConfig.UplinkOnly), 10)
		g.DownlinkOnly = strconv.FormatUint(uint64(cfg.ConnectionConfig.DownlinkOnly), 10)
		g.BufferSize = strconv.FormatInt(int64(cfg.ConnectionConfig.BufferSize), 10)
	}
	if cfg.Diagnostics != nil {
		g.DiagnosticsEnable, g.DiagnosticsListen = cfg.Diagnostics.Enable, cfg.Diagnostics.Listen
	}
	if cfg.Cache != nil {
		g.CacheEnable, g.CachePath, g.CacheMaxAge = cfg.Cache.Enable, cfg.Cache.Path, strconv.Itoa(cfg.Cache.MaxAge)
	}
	return g
}

func parseInt(value string) int { v, _ := strconv.Atoi(strings.TrimSpace(value)); return v }
func parseUint(value string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return v
}
func parseFloat(value string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return v
}
func formatFloat(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

func splitLines(value string) []string {
	var result []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == ',' }) {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func splitLinesKeepEmpty(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{""}
	}
	return splitLines(value)
}

func parseEnv(value string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == ',' }) {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func formatEnv(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var lines []string
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}
	return strings.Join(lines, "\n")
}

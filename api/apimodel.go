package api

import (
	"encoding/json"
	"regexp"

	"github.com/xtls/xray-core/infra/conf"
)

// Config API config
type Config struct {
	APIHost      string  `mapstructure:"ApiHost" json:"ApiHost" yaml:"ApiHost"`
	NodeID       int     `mapstructure:"NodeID" json:"NodeID" yaml:"NodeID"`
	Key          string  `mapstructure:"ApiKey" json:"ApiKey" yaml:"ApiKey"`
	NodeType     string  `mapstructure:"NodeType" json:"NodeType" yaml:"NodeType"`
	EnableVless  bool    `mapstructure:"EnableVless" json:"EnableVless" yaml:"EnableVless"`
	VlessFlow    string  `mapstructure:"VlessFlow" json:"VlessFlow,omitempty" yaml:"VlessFlow,omitempty"`
	Timeout      int     `mapstructure:"Timeout" json:"Timeout" yaml:"Timeout"`
	SpeedLimit   float64 `mapstructure:"SpeedLimit" json:"SpeedLimit" yaml:"SpeedLimit"`
	DeviceLimit  int     `mapstructure:"DeviceLimit" json:"DeviceLimit" yaml:"DeviceLimit"`
	RuleListPath string  `mapstructure:"RuleListPath" json:"RuleListPath,omitempty" yaml:"RuleListPath,omitempty"`
}

// NodeStatus Node status
type NodeStatus struct {
	CPU    float64
	Mem    float64
	Disk   float64
	Uptime uint64
	// Absolute resource figures in bytes; zero when unknown.
	MemTotal, MemUsed   uint64
	SwapTotal, SwapUsed uint64
	DiskTotal, DiskUsed uint64
	// Synchronization state, so the panel can show nodes whose latest
	// configuration was rejected while they keep serving the previous one.
	SyncError  string // empty when the last synchronization succeeded
	LastSyncAt int64  // unix seconds of the last successful sync, 0 if never
	Users      int    // users the node currently serves
	Goroutines int
}

type NodeInfo struct {
	AcceptProxyProtocol bool
	Authority           string
	NodeType            string // Must be V2ray, Trojan, and Shadowsocks
	NodeID              int
	Port                uint32
	SpeedLimit          uint64 // Bps
	AlterID             uint16
	TransportProtocol   string
	FakeType            string
	Host                string
	Path                string
	EnableTLS           bool
	EnableSniffing      bool
	RouteOnly           bool
	EnableVless         bool
	VlessFlow           string
	Decryption          string // VLESS inbound decryption, "none" unless the panel enables VLESS Encryption.
	CypherMethod        string
	ServerKey           string
	ServiceName         string
	Method              string
	Header              json.RawMessage
	XHTTPMode           string          // XHTTP mode (auto, packet-up, stream-up, stream-one); empty means auto.
	XHTTPExtra          json.RawMessage // Server-side XHTTP "extra" settings, client-only keys removed.
	HttpHeaders         map[string]*conf.StringList
	Headers             map[string]string
	NameServerConfig    []*conf.NameServerConfig
	EnableREALITY       bool
	REALITYConfig       *REALITYConfig
	Show                bool
	EnableTFO           bool
	Dest                string
	ProxyProtocolVer    uint64
	ServerNames         []string
	PrivateKey          string
	MinClientVer        string
	MaxClientVer        string
	MaxTimeDiff         uint64
	ShortIds            []string
	Xver                uint64
	Flow                string
	Security            string
	Key                 string
	RejectUnknownSni    bool
}

// PanelCertConfig is the non-secret part of a panel's per-node certificate
// config. DNS credentials and pushed certificate contents are never kept.
type PanelCertConfig struct {
	Mode     string // http, dns, file, self, content
	Domain   string
	Email    string
	Provider string // DNS provider, dns mode only
}

type UserInfo struct {
	UID         int
	Email       string
	UUID        string
	Passwd      string
	Port        uint32
	AlterID     uint16
	Method      string
	SpeedLimit  uint64 // Bps
	DeviceLimit int
}

type OnlineUser struct {
	UID int
	IP  string
}

type UserTraffic struct {
	UID      int
	Email    string
	Upload   int64
	Download int64
}

type ClientInfo struct {
	APIHost  string
	NodeID   int
	Key      string
	NodeType string
}

type DetectRule struct {
	ID      int
	Pattern *regexp.Regexp
}

type DetectResult struct {
	UID    int
	RuleID int
}

type REALITYConfig struct {
	Dest             string
	ProxyProtocolVer uint64
	ServerNames      []string
	PrivateKey       string
	MinClientVer     string
	MaxClientVer     string
	MaxTimeDiff      uint64
	ShortIds         []string
}

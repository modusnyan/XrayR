// Package api contains the interfaces implemented by panel API clients.
package api

// Client contains the operations required to initialize a node.
type Client interface {
	GetNodeInfo() (*NodeInfo, error)
	GetUserList() (*[]UserInfo, error)
	Describe() ClientInfo
	Debug()
}

// RuleProvider fetches audit rules supported by a panel.
type RuleProvider interface {
	GetNodeRule() (*[]DetectRule, error)
}

// IntervalsProvider exposes the sync intervals the panel wants the node to use
// (Xboard's base_config.push_interval / pull_interval, in seconds). A value <= 0
// means "not provided"; the controller then keeps its locally configured period.
type IntervalsProvider interface {
	// PullInterval is how often the node should pull node/user config.
	PullInterval() int
	// PushInterval is how often the node should report traffic and status.
	PushInterval() int
}

// TrafficReporter reports per-user traffic to a panel.
type TrafficReporter interface {
	ReportUserTraffic(*[]UserTraffic) error
}

// NodeStatusReporter reports host resource status to a panel.
type NodeStatusReporter interface {
	ReportNodeStatus(*NodeStatus) error
}

// OnlineUserReporter reports currently online users to a panel.
type OnlineUserReporter interface {
	ReportNodeOnlineUsers(*[]OnlineUser) error
}

// IllegalReporter reports audit-rule matches to a panel.
type IllegalReporter interface {
	ReportIllegal(*[]DetectResult) error
}

// CertConfigProvider exposes the certificate setup the panel describes for the
// node. XrayR does not apply it (TLS uses the local ControllerConfig.CertConfig);
// the controller only compares the two and warns about mismatches.
type CertConfigProvider interface {
	// PanelCertConfig returns nil when the panel sends no certificate config.
	PanelCertConfig() *PanelCertConfig
}

// Closer is implemented by clients with background resources.
type Closer interface {
	Close() error
}

// API is the legacy aggregate interface. New code should depend on Client and
// assert the optional capability interfaces above.
type API interface {
	Client
	RuleProvider
	TrafficReporter
	NodeStatusReporter
	OnlineUserReporter
	IllegalReporter
}

// PanelCapabilities describes operations and compatibility behavior exposed by
// a registered panel adapter.
type PanelCapabilities struct {
	Rules                        bool `json:"rules" yaml:"rules"`
	TrafficReport                bool `json:"traffic_report" yaml:"traffic_report"`
	NodeStatusReport             bool `json:"node_status_report" yaml:"node_status_report"`
	OnlineUserReport             bool `json:"online_user_report" yaml:"online_user_report"`
	IllegalReport                bool `json:"illegal_report" yaml:"illegal_report"`
	Shadowsocks2022KeyDerivation bool `json:"shadowsocks_2022_key_derivation" yaml:"shadowsocks_2022_key_derivation"`
}

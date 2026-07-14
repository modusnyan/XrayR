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

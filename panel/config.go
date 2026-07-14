package panel

import (
	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/service/controller"
)

type Config struct {
	ConfigVersion      int                `mapstructure:"ConfigVersion" json:"ConfigVersion" yaml:"ConfigVersion"`
	LogConfig          *LogConfig         `mapstructure:"Log" json:"Log,omitempty" yaml:"Log,omitempty"`
	DnsConfigPath      string             `mapstructure:"DnsConfigPath" json:"DnsConfigPath,omitempty" yaml:"DnsConfigPath,omitempty"`
	InboundConfigPath  string             `mapstructure:"InboundConfigPath" json:"InboundConfigPath,omitempty" yaml:"InboundConfigPath,omitempty"`
	OutboundConfigPath string             `mapstructure:"OutboundConfigPath" json:"OutboundConfigPath,omitempty" yaml:"OutboundConfigPath,omitempty"`
	RouteConfigPath    string             `mapstructure:"RouteConfigPath" json:"RouteConfigPath,omitempty" yaml:"RouteConfigPath,omitempty"`
	ConnectionConfig   *ConnectionConfig  `mapstructure:"ConnectionConfig" json:"ConnectionConfig,omitempty" yaml:"ConnectionConfig,omitempty"`
	Diagnostics        *DiagnosticsConfig `mapstructure:"Diagnostics" json:"Diagnostics,omitempty" yaml:"Diagnostics,omitempty"`
	Cache              *CacheConfig       `mapstructure:"Cache" json:"Cache,omitempty" yaml:"Cache,omitempty"`
	NodesConfig        []*NodesConfig     `mapstructure:"Nodes" json:"Nodes" yaml:"Nodes"`
}

type NodesConfig struct {
	PanelType        string             `mapstructure:"PanelType" json:"PanelType" yaml:"PanelType"`
	ApiConfig        *api.Config        `mapstructure:"ApiConfig" json:"ApiConfig" yaml:"ApiConfig"`
	ControllerConfig *controller.Config `mapstructure:"ControllerConfig" json:"ControllerConfig,omitempty" yaml:"ControllerConfig,omitempty"`
}

type DiagnosticsConfig struct {
	Enable bool   `mapstructure:"Enable" json:"Enable" yaml:"Enable"`
	Listen string `mapstructure:"Listen" json:"Listen" yaml:"Listen"`
}

type CacheConfig struct {
	Enable bool   `mapstructure:"Enable" json:"Enable" yaml:"Enable"`
	Path   string `mapstructure:"Path" json:"Path,omitempty" yaml:"Path,omitempty"`
	MaxAge int    `mapstructure:"MaxAge" json:"MaxAge" yaml:"MaxAge"`
}

type LogConfig struct {
	Level      string `mapstructure:"Level" json:"Level" yaml:"Level"`
	Format     string `mapstructure:"Format" json:"Format" yaml:"Format"`
	AccessPath string `mapstructure:"AccessPath" json:"AccessPath,omitempty" yaml:"AccessPath,omitempty"`
	ErrorPath  string `mapstructure:"ErrorPath" json:"ErrorPath,omitempty" yaml:"ErrorPath,omitempty"`
}

type ConnectionConfig struct {
	Handshake    uint32 `mapstructure:"handshake" json:"Handshake" yaml:"Handshake"`
	ConnIdle     uint32 `mapstructure:"connIdle" json:"ConnIdle" yaml:"ConnIdle"`
	UplinkOnly   uint32 `mapstructure:"uplinkOnly" json:"UplinkOnly" yaml:"UplinkOnly"`
	DownlinkOnly uint32 `mapstructure:"downlinkOnly" json:"DownlinkOnly" yaml:"DownlinkOnly"`
	BufferSize   int32  `mapstructure:"bufferSize" json:"BufferSize" yaml:"BufferSize"`
}

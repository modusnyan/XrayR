package controller

import (
	"github.com/XrayR-project/XrayR/common/limiter"
	"github.com/XrayR-project/XrayR/common/mylego"
)

type Config struct {
	SnapshotPath              string                           `mapstructure:"-" json:"-" yaml:"-"`
	SnapshotMaxAge            int                              `mapstructure:"-" json:"-" yaml:"-"`
	ListenIP                  string                           `mapstructure:"ListenIP" json:"ListenIP" yaml:"ListenIP"`
	SendIP                    string                           `mapstructure:"SendIP" json:"SendIP" yaml:"SendIP"`
	UpdatePeriodic            int                              `mapstructure:"UpdatePeriodic" json:"UpdatePeriodic" yaml:"UpdatePeriodic"`
	CertConfig                *mylego.CertConfig               `mapstructure:"CertConfig" json:"CertConfig,omitempty" yaml:"CertConfig,omitempty"`
	EnableDNS                 bool                             `mapstructure:"EnableDNS" json:"EnableDNS" yaml:"EnableDNS"`
	DNSType                   string                           `mapstructure:"DNSType" json:"DNSType" yaml:"DNSType"`
	DisableUploadTraffic      bool                             `mapstructure:"DisableUploadTraffic" json:"DisableUploadTraffic" yaml:"DisableUploadTraffic"`
	DisableGetRule            bool                             `mapstructure:"DisableGetRule" json:"DisableGetRule" yaml:"DisableGetRule"`
	EnableProxyProtocol       bool                             `mapstructure:"EnableProxyProtocol" json:"EnableProxyProtocol" yaml:"EnableProxyProtocol"`
	EnableFallback            bool                             `mapstructure:"EnableFallback" json:"EnableFallback" yaml:"EnableFallback"`
	DisableIVCheck            bool                             `mapstructure:"DisableIVCheck" json:"DisableIVCheck" yaml:"DisableIVCheck"`
	DisableSniffing           bool                             `mapstructure:"DisableSniffing" json:"DisableSniffing" yaml:"DisableSniffing"`
	AutoSpeedLimitConfig      *AutoSpeedLimitConfig            `mapstructure:"AutoSpeedLimitConfig" json:"AutoSpeedLimitConfig,omitempty" yaml:"AutoSpeedLimitConfig,omitempty"`
	GlobalDeviceLimitConfig   *limiter.GlobalDeviceLimitConfig `mapstructure:"GlobalDeviceLimitConfig" json:"GlobalDeviceLimitConfig,omitempty" yaml:"GlobalDeviceLimitConfig,omitempty"`
	FallBackConfigs           []*FallBackConfig                `mapstructure:"FallBackConfigs" json:"FallBackConfigs,omitempty" yaml:"FallBackConfigs,omitempty"`
	DisableLocalREALITYConfig bool                             `mapstructure:"DisableLocalREALITYConfig" json:"DisableLocalREALITYConfig" yaml:"DisableLocalREALITYConfig"`
	EnableREALITY             bool                             `mapstructure:"EnableREALITY" json:"EnableREALITY" yaml:"EnableREALITY"`
	REALITYConfigs            *REALITYConfig                   `mapstructure:"REALITYConfigs" json:"REALITYConfigs,omitempty" yaml:"REALITYConfigs,omitempty"`
}

type AutoSpeedLimitConfig struct {
	Limit         int `mapstructure:"Limit" json:"Limit" yaml:"Limit"`
	WarnTimes     int `mapstructure:"WarnTimes" json:"WarnTimes" yaml:"WarnTimes"`
	LimitSpeed    int `mapstructure:"LimitSpeed" json:"LimitSpeed" yaml:"LimitSpeed"`
	LimitDuration int `mapstructure:"LimitDuration" json:"LimitDuration" yaml:"LimitDuration"`
}

type FallBackConfig struct {
	SNI              string `mapstructure:"SNI" json:"SNI" yaml:"SNI"`
	Alpn             string `mapstructure:"Alpn" json:"Alpn" yaml:"Alpn"`
	Path             string `mapstructure:"Path" json:"Path" yaml:"Path"`
	Dest             string `mapstructure:"Dest" json:"Dest" yaml:"Dest"`
	ProxyProtocolVer uint64 `mapstructure:"ProxyProtocolVer" json:"ProxyProtocolVer" yaml:"ProxyProtocolVer"`
}

type REALITYConfig struct {
	Show             bool     `mapstructure:"Show" json:"Show" yaml:"Show"`
	Dest             string   `mapstructure:"Dest" json:"Dest" yaml:"Dest"`
	ProxyProtocolVer uint64   `mapstructure:"ProxyProtocolVer" json:"ProxyProtocolVer" yaml:"ProxyProtocolVer"`
	ServerNames      []string `mapstructure:"ServerNames" json:"ServerNames" yaml:"ServerNames"`
	PrivateKey       string   `mapstructure:"PrivateKey" json:"PrivateKey" yaml:"PrivateKey"`
	MinClientVer     string   `mapstructure:"MinClientVer" json:"MinClientVer,omitempty" yaml:"MinClientVer,omitempty"`
	MaxClientVer     string   `mapstructure:"MaxClientVer" json:"MaxClientVer,omitempty" yaml:"MaxClientVer,omitempty"`
	MaxTimeDiff      uint64   `mapstructure:"MaxTimeDiff" json:"MaxTimeDiff" yaml:"MaxTimeDiff"`
	ShortIds         []string `mapstructure:"ShortIds" json:"ShortIds" yaml:"ShortIds"`
}

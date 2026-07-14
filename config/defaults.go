package config

import (
	"path/filepath"

	"github.com/XrayR-project/XrayR/common/limiter"
	"github.com/XrayR-project/XrayR/panel"
	"github.com/XrayR-project/XrayR/service/controller"
)

// ApplyDefaults fills omitted values without overriding explicit user input.
func ApplyDefaults(cfg *panel.Config, configDir string) {
	if cfg.LogConfig == nil {
		cfg.LogConfig = &panel.LogConfig{}
	}
	if cfg.LogConfig.Level == "" {
		cfg.LogConfig.Level = "none"
	}
	if cfg.LogConfig.Format == "" {
		cfg.LogConfig.Format = "text"
	}
	if cfg.ConnectionConfig == nil {
		cfg.ConnectionConfig = &panel.ConnectionConfig{}
	}
	if cfg.ConnectionConfig.Handshake == 0 {
		cfg.ConnectionConfig.Handshake = 4
	}
	if cfg.ConnectionConfig.ConnIdle == 0 {
		cfg.ConnectionConfig.ConnIdle = 30
	}
	if cfg.ConnectionConfig.UplinkOnly == 0 {
		cfg.ConnectionConfig.UplinkOnly = 2
	}
	if cfg.ConnectionConfig.DownlinkOnly == 0 {
		cfg.ConnectionConfig.DownlinkOnly = 4
	}
	if cfg.ConnectionConfig.BufferSize == 0 {
		cfg.ConnectionConfig.BufferSize = 64
	}
	if cfg.Diagnostics == nil {
		cfg.Diagnostics = &panel.DiagnosticsConfig{}
	}
	if cfg.Diagnostics.Listen == "" {
		cfg.Diagnostics.Listen = "127.0.0.1:8080"
	}
	if cfg.Cache == nil {
		cfg.Cache = &panel.CacheConfig{Enable: true}
	}
	if cfg.Cache.Path == "" {
		cfg.Cache.Path = filepath.Join(configDir, "cache")
	}
	if cfg.Cache.MaxAge == 0 {
		cfg.Cache.MaxAge = 24 * 60 * 60
	}

	for _, node := range cfg.NodesConfig {
		if node == nil {
			continue
		}
		if node.ApiConfig != nil {
			if node.ApiConfig.Timeout == 0 {
				node.ApiConfig.Timeout = 5
			}
			if node.ApiConfig.VlessFlow == "" {
				node.ApiConfig.VlessFlow = "xtls-rprx-vision"
			}
		}
		if node.ControllerConfig == nil {
			node.ControllerConfig = &controller.Config{}
		}
		controllerCfg := node.ControllerConfig
		if controllerCfg.ListenIP == "" {
			controllerCfg.ListenIP = "0.0.0.0"
		}
		if controllerCfg.SendIP == "" {
			controllerCfg.SendIP = "0.0.0.0"
		}
		if controllerCfg.UpdatePeriodic == 0 {
			controllerCfg.UpdatePeriodic = 60
		}
		if controllerCfg.DNSType == "" {
			controllerCfg.DNSType = "AsIs"
		}
		if controllerCfg.AutoSpeedLimitConfig == nil {
			controllerCfg.AutoSpeedLimitConfig = &controller.AutoSpeedLimitConfig{}
		}
		if controllerCfg.GlobalDeviceLimitConfig == nil {
			controllerCfg.GlobalDeviceLimitConfig = &limiter.GlobalDeviceLimitConfig{}
		}
	}
}

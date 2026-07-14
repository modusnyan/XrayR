package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"dario.cat/mergo"
	"github.com/r3labs/diff/v2"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"

	"github.com/XrayR-project/XrayR/app/mydispatcher"
	_ "github.com/XrayR-project/XrayR/cmd/distro/all"
	"github.com/XrayR-project/XrayR/service"
	"github.com/XrayR-project/XrayR/service/controller"
	"github.com/XrayR-project/XrayR/service/diagnostics"
)

// DiagnosticStatus returns sanitized runtime state for all controllers.
func (p *Panel) DiagnosticStatus() []diagnostics.NodeStatus {
	p.statusMu.RLock()
	defer p.statusMu.RUnlock()
	statuses := make([]diagnostics.NodeStatus, 0, len(p.Service))
	for _, runningService := range p.Service {
		if provider, ok := runningService.(service.StatusProvider); ok {
			statuses = append(statuses, provider.DiagnosticStatus())
		}
	}
	return statuses
}

// Panel Structure
type Panel struct {
	access      sync.Mutex
	statusMu    sync.RWMutex
	panelConfig *Config
	Server      *core.Instance
	Service     []service.Service
	Running     bool
	diagnostics *diagnostics.Server
}

func New(panelConfig *Config) *Panel {
	p := &Panel{panelConfig: panelConfig}
	return p
}

func (p *Panel) loadCore(panelConfig *Config) (*core.Instance, error) {
	// Log Config
	coreLogConfig := &conf.LogConfig{}
	logConfig := getDefaultLogConfig()
	if panelConfig.LogConfig != nil {
		if _, err := diff.Merge(logConfig, panelConfig.LogConfig, logConfig); err != nil {
			return nil, fmt.Errorf("read log config: %w", err)
		}
	}
	coreLogConfig.LogLevel = logConfig.Level
	coreLogConfig.AccessLog = logConfig.AccessPath
	coreLogConfig.ErrorLog = logConfig.ErrorPath

	// DNS config
	coreDnsConfig := &conf.DNSConfig{}
	if panelConfig.DnsConfigPath != "" {
		if data, err := os.ReadFile(panelConfig.DnsConfigPath); err != nil {
			return nil, fmt.Errorf("read DNS config %s: %w", panelConfig.DnsConfigPath, err)
		} else {
			if err = json.Unmarshal(data, coreDnsConfig); err != nil {
				return nil, fmt.Errorf("parse DNS config %s: %w", panelConfig.DnsConfigPath, err)
			}
		}
	}

	// init controller's DNS config
	// for _, config := range p.panelConfig.NodesConfig {
	// 	config.ControllerConfig.DNSConfig = coreDnsConfig
	// }

	dnsConfig, err := coreDnsConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build DNS config: %w", err)
	}

	// Routing config
	coreRouterConfig := &conf.RouterConfig{}
	if panelConfig.RouteConfigPath != "" {
		if data, err := os.ReadFile(panelConfig.RouteConfigPath); err != nil {
			return nil, fmt.Errorf("read routing config %s: %w", panelConfig.RouteConfigPath, err)
		} else {
			if err = json.Unmarshal(data, coreRouterConfig); err != nil {
				return nil, fmt.Errorf("parse routing config %s: %w", panelConfig.RouteConfigPath, err)
			}
		}
	}
	routeConfig, err := coreRouterConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build routing config: %w", err)
	}
	// Custom Inbound config
	var coreCustomInboundConfig []conf.InboundDetourConfig
	if panelConfig.InboundConfigPath != "" {
		if data, err := os.ReadFile(panelConfig.InboundConfigPath); err != nil {
			return nil, fmt.Errorf("read custom inbound config %s: %w", panelConfig.InboundConfigPath, err)
		} else {
			if err = json.Unmarshal(data, &coreCustomInboundConfig); err != nil {
				return nil, fmt.Errorf("parse custom inbound config %s: %w", panelConfig.InboundConfigPath, err)
			}
		}
	}
	var inBoundConfig []*core.InboundHandlerConfig
	for _, config := range coreCustomInboundConfig {
		oc, err := config.Build()
		if err != nil {
			return nil, fmt.Errorf("build custom inbound config: %w", err)
		}
		inBoundConfig = append(inBoundConfig, oc)
	}
	// Custom Outbound config
	var coreCustomOutboundConfig []conf.OutboundDetourConfig
	if panelConfig.OutboundConfigPath != "" {
		if data, err := os.ReadFile(panelConfig.OutboundConfigPath); err != nil {
			return nil, fmt.Errorf("read custom outbound config %s: %w", panelConfig.OutboundConfigPath, err)
		} else {
			if err = json.Unmarshal(data, &coreCustomOutboundConfig); err != nil {
				return nil, fmt.Errorf("parse custom outbound config %s: %w", panelConfig.OutboundConfigPath, err)
			}
		}
	}
	var outBoundConfig []*core.OutboundHandlerConfig
	for _, config := range coreCustomOutboundConfig {
		oc, err := config.Build()
		if err != nil {
			return nil, fmt.Errorf("build custom outbound config: %w", err)
		}
		outBoundConfig = append(outBoundConfig, oc)
	}
	// Policy config
	levelPolicyConfig := parseConnectionConfig(panelConfig.ConnectionConfig)
	corePolicyConfig := &conf.PolicyConfig{}
	corePolicyConfig.Levels = map[uint32]*conf.Policy{0: levelPolicyConfig}
	policyConfig, _ := corePolicyConfig.Build()
	// Build Core Config
	config := &core.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(coreLogConfig.Build()),
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&mydispatcher.Config{}),
			serial.ToTypedMessage(&stats.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
			serial.ToTypedMessage(policyConfig),
			serial.ToTypedMessage(dnsConfig),
			serial.ToTypedMessage(routeConfig),
		},
		Inbound:  inBoundConfig,
		Outbound: outBoundConfig,
	}
	server, err := core.New(config)
	if err != nil {
		return nil, fmt.Errorf("create core instance: %w", err)
	}

	return server, nil
}

// Start the panel
func (p *Panel) Start() error {
	p.access.Lock()
	defer p.access.Unlock()
	log.Print("Start the panel..")
	server, err := p.loadCore(p.panelConfig)
	if err != nil {
		return err
	}
	if err := server.Start(); err != nil {
		return fmt.Errorf("start core instance: %w", err)
	}
	p.Server = server

	for _, nodeConfig := range p.panelConfig.NodesConfig {
		definition, err := LookupPanel(nodeConfig.PanelType)
		if err != nil {
			_ = p.closeUnlocked()
			return err
		}
		apiClient := definition.New(nodeConfig.ApiConfig)
		controllerConfig := getDefaultControllerConfig()
		if nodeConfig.ControllerConfig != nil {
			if err := mergo.Merge(controllerConfig, nodeConfig.ControllerConfig, mergo.WithOverride); err != nil {
				_ = p.closeUnlocked()
				return fmt.Errorf("merge controller config: %w", err)
			}
		}
		if p.panelConfig.Cache != nil && p.panelConfig.Cache.Enable {
			controllerConfig.SnapshotPath = p.panelConfig.Cache.Path
			controllerConfig.SnapshotMaxAge = p.panelConfig.Cache.MaxAge
		}
		p.Service = append(p.Service, controller.New(server, apiClient, controllerConfig, definition.Adapter, definition.Capabilities))
	}

	for _, runningService := range p.Service {
		if err := runningService.Start(); err != nil {
			_ = p.closeUnlocked()
			return fmt.Errorf("start panel service: %w", err)
		}
	}
	p.Running = true
	if p.panelConfig.Diagnostics != nil && p.panelConfig.Diagnostics.Enable {
		p.diagnostics = diagnostics.New(p.panelConfig.Diagnostics.Listen, p)
		if err := p.diagnostics.Start(); err != nil {
			_ = p.closeUnlocked()
			return fmt.Errorf("start diagnostics: %w", err)
		}
	}
	return nil
}

// Close the panel
func (p *Panel) Close() error {
	p.access.Lock()
	defer p.access.Unlock()
	return p.closeUnlocked()
}

func (p *Panel) closeUnlocked() error {
	var closeErr error
	if p.diagnostics != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := p.diagnostics.Close(ctx); err != nil {
			closeErr = err
		}
		cancel()
		p.diagnostics = nil
	}
	for _, runningService := range p.Service {
		if err := runningService.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	p.Service = nil
	if p.Server != nil {
		if err := p.Server.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		p.Server = nil
	}
	p.Running = false
	return closeErr
}

func parseConnectionConfig(c *ConnectionConfig) (policy *conf.Policy) {
	connectionConfig := getDefaultConnectionConfig()
	if c != nil {
		if _, err := diff.Merge(connectionConfig, c, connectionConfig); err != nil {
			log.Panicf("Read ConnectionConfig failed: %s", err)
		}
	}
	policy = &conf.Policy{
		StatsUserUplink:   true,
		StatsUserDownlink: true,
		Handshake:         &connectionConfig.Handshake,
		ConnectionIdle:    &connectionConfig.ConnIdle,
		UplinkOnly:        &connectionConfig.UplinkOnly,
		DownlinkOnly:      &connectionConfig.DownlinkOnly,
		BufferSize:        &connectionConfig.BufferSize,
	}

	return
}

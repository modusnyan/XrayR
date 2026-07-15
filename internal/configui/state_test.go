package configui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/common/limiter"
	"github.com/XrayR-project/XrayR/common/mylego"
	"github.com/XrayR-project/XrayR/panel"
	"github.com/XrayR-project/XrayR/service/controller"
)

func TestStateRoundTripFullConfig(t *testing.T) {
	cfg := &panel.Config{
		ConfigVersion:    1,
		LogConfig:        &panel.LogConfig{Level: "info", Format: "json", AccessPath: "/tmp/access.log"},
		ConnectionConfig: &panel.ConnectionConfig{Handshake: 5, ConnIdle: 40, UplinkOnly: 3, DownlinkOnly: 6, BufferSize: 128},
		Diagnostics:      &panel.DiagnosticsConfig{Enable: true, Listen: "127.0.0.1:9090"},
		Cache:            &panel.CacheConfig{Enable: true, Path: "/tmp/cache", MaxAge: 7200},
		NodesConfig: []*panel.NodesConfig{{
			PanelType: "Xboard",
			ApiConfig: &api.Config{APIHost: "https://panel.example.com", Key: "secret", NodeID: 7, NodeType: "Vless", Timeout: 30, VlessFlow: "xtls-rprx-vision"},
			ControllerConfig: &controller.Config{
				ListenIP: "0.0.0.0", SendIP: "0.0.0.0", UpdatePeriodic: 60, DNSType: "AsIs",
				EnableREALITY:           true,
				REALITYConfigs:          &controller.REALITYConfig{Dest: "example.com:443", ServerNames: []string{"example.com"}, PrivateKey: "private", ShortIds: []string{"aabb"}},
				CertConfig:              &mylego.CertConfig{CertMode: "dns", CertDomain: "node.example.com", Provider: "cloudflare", Email: "ops@example.com", DNSEnv: map[string]string{"CF_DNS_API_TOKEN": "token"}},
				GlobalDeviceLimitConfig: &limiter.GlobalDeviceLimitConfig{Enable: true, RedisNetwork: "tcp", RedisAddr: "127.0.0.1:6379", Timeout: 5, Expiry: 60},
				EnableFallback:          true, FallBackConfigs: []*controller.FallBackConfig{{Dest: "80", SNI: "example.com"}},
			},
		}},
	}

	state := StateFromConfig(cfg, t.TempDir())
	roundTrip, err := state.Config(t.TempDir())
	require.NoError(t, err)
	require.Len(t, roundTrip.NodesConfig, 1)
	assert.Equal(t, "secret", roundTrip.NodesConfig[0].ApiConfig.Key)
	assert.Equal(t, "token", roundTrip.NodesConfig[0].ControllerConfig.CertConfig.DNSEnv["CF_DNS_API_TOKEN"])
	assert.Equal(t, "private", roundTrip.NodesConfig[0].ControllerConfig.REALITYConfigs.PrivateKey)
	assert.Equal(t, "80", roundTrip.NodesConfig[0].ControllerConfig.FallBackConfigs[0].Dest)
	assert.True(t, roundTrip.Diagnostics.Enable)
	assert.Equal(t, "json", roundTrip.LogConfig.Format)
}

func TestCloneNodeDeepCopiesFallbacks(t *testing.T) {
	state := NewState(t.TempDir())
	state.Nodes[0].NodeID = "1"
	state.Nodes[0].Fallbacks[0].Dest = "80"
	index := state.CloneNode(0)
	require.Equal(t, 1, index)
	state.Nodes[index].Fallbacks[0].Dest = "8080"
	assert.Equal(t, "80", state.Nodes[0].Fallbacks[0].Dest)
	assert.Empty(t, state.Nodes[index].NodeID)
}

func TestParseEnvAndSplitLines(t *testing.T) {
	assert.Equal(t, map[string]string{"A": "one", "B": "two=more"}, parseEnv("A=one\nB=two=more"))
	assert.Equal(t, []string{"a", "b", "c"}, splitLines("a,b\nc"))
	assert.Equal(t, []string{""}, splitLinesKeepEmpty(""))
}

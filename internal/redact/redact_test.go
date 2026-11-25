package redact

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

func TestURLRemovesQueryAndUserInfo(t *testing.T) {
	assert.Equal(t, "https://panel.example.com/api",
		URL("https://user:pass@panel.example.com/api?token=secret&node=1"))

	// A value that is not a URL still loses anything after the first '?'.
	assert.Equal(t, "/api", URL("/api?token=secret"))
}

func TestConfigMasksEverySecret(t *testing.T) {
	source := &panel.Config{
		NodesConfig: []*panel.NodesConfig{{
			PanelType: "Xboard",
			ApiConfig: &api.Config{
				APIHost: "https://panel.example.com/api?token=secret",
				Key:     "api-key-value",
				NodeID:  12,
			},
			ControllerConfig: &controller.Config{
				GlobalDeviceLimitConfig: &limiter.GlobalDeviceLimitConfig{
					Enable:        true,
					RedisAddr:     "127.0.0.1:6379",
					RedisPassword: "redis-password",
				},
				REALITYConfigs: &controller.REALITYConfig{
					Dest:       "www.example.com:443",
					PrivateKey: "reality-private-key",
				},
				CertConfig: &mylego.CertConfig{
					CertMode:   "dns",
					CertDomain: "node.example.com",
					DNSEnv:     map[string]string{"ALICLOUD_SECRET_KEY": "dns-secret"},
				},
			},
		}},
	}

	view := Config(source)

	require.Len(t, view.NodesConfig, 1)
	node := view.NodesConfig[0]
	assert.Equal(t, Mask, node.ApiConfig.Key)
	assert.Equal(t, "https://panel.example.com/api", node.ApiConfig.APIHost)
	assert.Equal(t, 12, node.ApiConfig.NodeID, "non-secret fields are preserved")

	assert.Equal(t, Mask, node.ControllerConfig.GlobalDeviceLimitConfig.RedisPassword)
	assert.Equal(t, "127.0.0.1:6379", node.ControllerConfig.GlobalDeviceLimitConfig.RedisAddr)
	assert.Equal(t, Mask, node.ControllerConfig.REALITYConfigs.PrivateKey)
	assert.Equal(t, Mask, node.ControllerConfig.CertConfig.DNSEnv["ALICLOUD_SECRET_KEY"])

	// The original configuration must not be modified: it is still in use at runtime.
	assert.Equal(t, "api-key-value", source.NodesConfig[0].ApiConfig.Key)
	assert.Equal(t, "redis-password", source.NodesConfig[0].ControllerConfig.GlobalDeviceLimitConfig.RedisPassword)
	assert.Equal(t, "reality-private-key", source.NodesConfig[0].ControllerConfig.REALITYConfigs.PrivateKey)
	assert.Equal(t, "dns-secret", source.NodesConfig[0].ControllerConfig.CertConfig.DNSEnv["ALICLOUD_SECRET_KEY"])
	assert.Equal(t, "https://panel.example.com/api?token=secret", source.NodesConfig[0].ApiConfig.APIHost)
}

func TestConfigHandlesNilAndEmptyValues(t *testing.T) {
	assert.Nil(t, Config(nil))

	view := Config(&panel.Config{NodesConfig: []*panel.NodesConfig{nil, {PanelType: "Xboard"}}})
	require.Len(t, view.NodesConfig, 2)
	assert.Nil(t, view.NodesConfig[0])
	assert.Nil(t, view.NodesConfig[1].ApiConfig)
}

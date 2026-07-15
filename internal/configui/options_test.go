package configui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeTypeOptionsFollowPanelRegistry(t *testing.T) {
	options := nodeTypeOptions("Xboard")
	var values []string
	for _, option := range options {
		values = append(values, option.Value)
	}
	assert.Contains(t, values, "Vless")
	assert.NotContains(t, values, "Shadowsocks-Plugin")
}

func TestFieldValidators(t *testing.T) {
	require.NoError(t, validateURL("https://panel.example.com"))
	assert.Error(t, validateURL("panel.example.com"))
	require.NoError(t, validatePositiveInt("1"))
	assert.Error(t, validatePositiveInt("0"))
	require.NoError(t, validateEnv("KEY=value\nOTHER=value"))
	assert.Error(t, validateEnv("missing-separator"))
}

func TestConditionalHelpers(t *testing.T) {
	assert.True(t, isVLESS("Vless", false))
	assert.True(t, isVLESS("V2ray", true))
	assert.False(t, isVLESS("Vmess", false))
	assert.True(t, supportsFallback("Trojan", false))
	assert.True(t, supportsFallback("V2ray", true))
	assert.False(t, supportsFallback("Shadowsocks", false))
}

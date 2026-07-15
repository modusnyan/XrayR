package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeRejectsUnknownFields(t *testing.T) {
	_, err := Decode(strings.NewReader(`
ConfigVersion: 1
UnknownField: true
Nodes: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UnknownField")
}

func TestDecodeWarnsAboutMachineID(t *testing.T) {
	result, err := Decode(strings.NewReader(`
Nodes:
  - PanelType: Xboard
    ApiConfig:
      ApiHost: https://example.com
      ApiKey: key
      NodeID: 1
      MachineID: 2
      NodeType: Vless
`))
	require.NoError(t, err)
	require.Len(t, result.Issues, 1)
	assert.Equal(t, SeverityWarning, result.Issues[0].Severity)
}

func TestValidateReportsAllRequiredFields(t *testing.T) {
	result, err := Decode(strings.NewReader(`Nodes:
  - PanelType: Xbord
    ApiConfig:
      ApiHost: nope
      ApiKey: ""
      NodeID: 0
      NodeType: Unknown
`))
	require.NoError(t, err)
	ApplyDefaults(result.Config, t.TempDir())
	issues := Validate(result.Config)
	assert.GreaterOrEqual(t, len(issues), 4)
}

func TestValidateCrossFieldRequirements(t *testing.T) {
	result, err := Decode(strings.NewReader(`
ConfigVersion: 1
Nodes:
  - PanelType: Xboard
    ApiConfig:
      ApiHost: https://panel.example.com
      ApiKey: key
      NodeID: 1
      NodeType: Vless
    ControllerConfig:
      EnableFallback: true
      FallBackConfigs: []
      EnableREALITY: true
      REALITYConfigs:
        Dest: ""
        ServerNames: []
        PrivateKey: ""
      CertConfig:
        CertMode: dns
        CertDomain: ""
        Provider: ""
      GlobalDeviceLimitConfig:
        Enable: true
        RedisNetwork: udp
        RedisAddr: ""
        Timeout: 0
        Expiry: 0
      AutoSpeedLimitConfig:
        Limit: 10
        WarnTimes: -1
        LimitSpeed: 0
        LimitDuration: 0
`))
	require.NoError(t, err)
	ApplyDefaults(result.Config, t.TempDir())
	issues := Validate(result.Config)
	paths := make([]string, 0, len(issues))
	for _, issue := range issues {
		paths = append(paths, issue.Path)
	}
	assert.Contains(t, paths, "Nodes[0].ControllerConfig.FallBackConfigs")
	assert.Contains(t, paths, "Nodes[0].ControllerConfig.REALITYConfigs")
	assert.Contains(t, paths, "Nodes[0].ControllerConfig.CertConfig.CertDomain")
	assert.Contains(t, paths, "Nodes[0].ControllerConfig.CertConfig.Provider")
	assert.Contains(t, paths, "Nodes[0].ControllerConfig.GlobalDeviceLimitConfig.RedisNetwork")
	assert.Contains(t, paths, "Nodes[0].ControllerConfig.AutoSpeedLimitConfig.WarnTimes")
	assert.Contains(t, paths, "Nodes[0].ControllerConfig.AutoSpeedLimitConfig.LimitSpeed")
	assert.Contains(t, paths, "Nodes[0].ControllerConfig.AutoSpeedLimitConfig.LimitDuration")
}

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

package config

import (
	"os"
	"path/filepath"
	"regexp"
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

func TestDecodeExplainsMisplacedField(t *testing.T) {
	// CertConfig is a valid field, just one level too high: it belongs under
	// ControllerConfig, not directly under the node.
	_, err := Decode(strings.NewReader(`ConfigVersion: 1
Nodes:
  - PanelType: "Xboard"
    ApiConfig:
      ApiHost: "https://panel.example.com"
      ApiKey: key
      NodeID: 1
      NodeType: Vmess
    ControllerConfig:
      ListenIP: 0.0.0.0
    CertConfig:
      CertMode: none
`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "field CertConfig not found in type panel.NodesConfig")
	assert.Contains(t, err.Error(), "nest it under Nodes[].ControllerConfig")
}

func TestDecodeReportsTheLineInTheInputFile(t *testing.T) {
	// CertConfig is on line 11 of this document. Re-encoding the YAML before decoding
	// would sort the keys and make this line number point somewhere else.
	_, err := Decode(strings.NewReader(`ConfigVersion: 1
Nodes:
  - PanelType: "Xboard"
    ApiConfig:
      ApiHost: "https://panel.example.com"
      ApiKey: key
      NodeID: 1
      NodeType: Vmess
    ControllerConfig:
      ListenIP: 0.0.0.0
    CertConfig:
      CertMode: none
`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 11:")
}

func TestDecodeAcceptsCertConfigUnderControllerConfig(t *testing.T) {
	_, err := Decode(strings.NewReader(`ConfigVersion: 1
Nodes:
  - PanelType: "Xboard"
    ApiConfig:
      ApiHost: "https://panel.example.com"
      ApiKey: key
      NodeID: 1
      NodeType: Vmess
    ControllerConfig:
      ListenIP: 0.0.0.0
      CertConfig:
        CertMode: dns
        CertDomain: node.example.com
        Provider: cloudflare
`))

	assert.NoError(t, err)
}

// optionalSettingLine matches a commented-out YAML mapping inside the template's
// optional blocks, e.g. `#      CertConfig:` or `#        - SNI:`. Separator lines such
// as `# ----` and prose comments deliberately do not match.
var optionalSettingLine = regexp.MustCompile(`^#(\s*)([A-Za-z_][A-Za-z0-9_]*:.*|- .*)$`)

// uncommentOptionalBlocks strips the leading # from the real settings in the template's
// two "可选配置" sections, leaving prose and separators commented.
func uncommentOptionalBlocks(template string) (string, int) {
	var out []string
	inBlock := false
	uncommented := 0
	for _, line := range strings.Split(template, "\n") {
		switch {
		case strings.Contains(line, "可选配置"):
			inBlock = true
		case inBlock && strings.HasPrefix(line, "# ===="):
			inBlock = false
		}
		if inBlock {
			if match := optionalSettingLine.FindStringSubmatch(line); match != nil {
				out = append(out, match[1]+match[2])
				uncommented++
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), uncommented
}

// TestConfigTemplateOptionalBlocksStayValid guards the promise the shipped template
// makes: "to enable an option, delete the leading # and nothing else". The indentation
// inside those comments is the real indentation, so removing the # has to produce valid
// YAML at the correct nesting level — that is exactly what a hand-written config gets
// wrong (CertConfig written next to ControllerConfig instead of inside it).
func TestConfigTemplateOptionalBlocksStayValid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "release", "config", "config.template.yml"))
	require.NoError(t, err)

	uncommented, count := uncommentOptionalBlocks(string(raw))
	require.Greater(t, count, 20, "expected the optional blocks to contain real settings")

	result, err := Decode(strings.NewReader(uncommented))
	require.NoError(t, err, "uncommenting the template's optional blocks must still decode")

	require.Len(t, result.Config.NodesConfig, 1)
	node := result.Config.NodesConfig[0]

	// The node-level options must land inside ControllerConfig, not beside it.
	require.NotNil(t, node.ControllerConfig, "node options did not land in ControllerConfig")
	assert.NotNil(t, node.ControllerConfig.CertConfig, "CertConfig is not nested correctly")
	assert.Equal(t, "dns", node.ControllerConfig.CertConfig.CertMode)
	assert.Equal(t, "node1.example.com", node.ControllerConfig.CertConfig.CertDomain)
	assert.NotNil(t, node.ControllerConfig.REALITYConfigs)
	assert.NotNil(t, node.ControllerConfig.GlobalDeviceLimitConfig)
	assert.NotNil(t, node.ControllerConfig.AutoSpeedLimitConfig)
	assert.NotEmpty(t, node.ControllerConfig.FallBackConfigs)

	// ...and the top-level options must land at the top level.
	require.NotNil(t, result.Config.ConnectionConfig, "top-level options did not land at the top level")
	assert.EqualValues(t, 4, result.Config.ConnectionConfig.Handshake)
	require.NotNil(t, result.Config.Cache)
	assert.True(t, result.Config.Cache.Enable)
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

// TestValidateWarnsAboutTemplatePlaceholders keeps the promise the installed
// template makes: leaving the example values in place is reported, instead of
// starting a node that only fails later with an unreachable-host error.
func TestValidateWarnsAboutTemplatePlaceholders(t *testing.T) {
	result, err := Decode(strings.NewReader(`
ConfigVersion: 1
Nodes:
  - PanelType: Xboard
    ApiConfig:
      ApiHost: https://panel.example.com/
      ApiKey: CHANGE_ME
      NodeID: 1
      NodeType: Vmess
`))
	require.NoError(t, err)
	ApplyDefaults(result.Config, t.TempDir())

	paths := make([]string, 0)
	for _, issue := range Validate(result.Config) {
		if issue.Severity == SeverityWarning {
			paths = append(paths, issue.Path)
		}
	}
	assert.Contains(t, paths, "Nodes[0].ApiConfig.ApiHost")
	assert.Contains(t, paths, "Nodes[0].ApiConfig.ApiKey")
}

// TestValidateAcceptsRealPanelValues guards against the placeholder check firing
// on an edited configuration.
func TestValidateAcceptsRealPanelValues(t *testing.T) {
	result, err := Decode(strings.NewReader(`
ConfigVersion: 1
Nodes:
  - PanelType: Xboard
    ApiConfig:
      ApiHost: https://panel.my-domain.net
      ApiKey: 7f3c9a2b1d
      NodeID: 4
      NodeType: Vmess
`))
	require.NoError(t, err)
	ApplyDefaults(result.Config, t.TempDir())

	for _, issue := range Validate(result.Config) {
		if strings.Contains(issue.Message, "template") {
			t.Errorf("unexpected placeholder warning for a real configuration: %s", issue.Message)
		}
	}
}

package configui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/XrayR-project/XrayR/internal/redact"
)

func TestReviewPreviewRedactsSecrets(t *testing.T) {
	state := NewState(t.TempDir())
	node := &state.Nodes[0]
	node.APIHost = "https://panel.example.com?token=query-secret"
	node.APIKey = "api-secret"
	node.NodeID = "1"
	node.RedisEnable = true
	node.RedisPassword = "redis-secret"
	node.EnableREALITY = true
	node.RealityPrivateKey = "reality-secret"
	node.CertMode = "dns"
	node.CertDomain = "node.example.com"
	node.CertProvider = "cloudflare"
	node.CertEmail = "ops@example.com"
	node.CertDNSEnv = "CF_DNS_API_TOKEN=dns-secret"

	cfg, err := state.Config(t.TempDir())
	require.NoError(t, err)
	data, err := yaml.Marshal(redact.Config(cfg))
	require.NoError(t, err)
	output := string(data)
	for _, secret := range []string{"api-secret", "redis-secret", "reality-secret", "dns-secret", "query-secret"} {
		assert.NotContains(t, output, secret)
	}
	assert.True(t, strings.Contains(output, "***REDACTED***"))
}

func TestDiffText(t *testing.T) {
	diff := DiffText("a\nb\n", "a\nc\n")
	assert.Contains(t, diff, "-b")
	assert.Contains(t, diff, "+c")
}

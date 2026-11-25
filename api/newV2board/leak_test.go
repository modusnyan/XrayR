package newV2board

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/XrayR-project/XrayR/api"
)

// TestNodeConfigErrorsNeverLeakTheResponse makes sure a config XrayR rejects is
// reported without echoing the panel response, which contains node secrets.
func TestNodeConfigErrorsNeverLeakTheResponse(t *testing.T) {
	const secret = "SECRET-SERVER-KEY-0123456789"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"server_port":8388,"cipher":"2022-blake3-aes-128-gcm","server_key":"` + secret +
			`","plugin":"v2ray-plugin","plugin_opts":"mode=websocket","base_config":{"push_interval":60,"pull_interval":60}}`))
	}))
	t.Cleanup(server.Close)
	client := New(&api.Config{APIHost: server.URL, Key: "token", NodeID: 5, NodeType: "Shadowsocks", Timeout: 2})
	client.Client.SetRetryCount(0)

	_, err := client.GetNodeInfo()
	if err == nil {
		t.Fatal("an unsupported plugin must be rejected")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks the panel response: %v", err)
	}
	if !strings.Contains(err.Error(), "unsupported shadowsocks plugin") {
		t.Fatalf("error lost the reason: %v", err)
	}
}

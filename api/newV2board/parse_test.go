package newV2board

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/XrayR-project/XrayR/api"
)

func testClient(nodeType string) *APIClient {
	return New(&api.Config{APIHost: "http://panel.invalid", Key: "token", NodeID: 3, NodeType: nodeType})
}

func ssConfig(plugin, opts string) *serverConfig {
	s := &serverConfig{ServerPort: 8388}
	s.Cipher = "aes-128-gcm"
	s.Plugin = plugin
	s.PluginOpts = opts
	return s
}

func obfsPath(t *testing.T, node *api.NodeInfo) string {
	t.Helper()
	if node.Header == nil {
		t.Fatal("expected an http obfs header")
	}
	var header struct {
		Type    string `json:"type"`
		Request struct {
			// xray-core's AuthenticatorRequest.Path is a StringList that
			// accepts a bare string as well as an array; the adapter writes
			// the bare string, so only well-formedness is checked here.
			Path json.RawMessage `json:"path"`
		} `json:"request"`
	}
	if err := json.Unmarshal(node.Header, &header); err != nil {
		t.Fatalf("header is not valid JSON: %v", err)
	}
	if header.Type != "http" {
		t.Fatalf("header type = %q, want http", header.Type)
	}
	return string(node.Header)
}

func TestSSPluginObfsHTTPBuildsTheHeader(t *testing.T) {
	node, err := testClient("Shadowsocks").parseSSNodeResponse(ssConfig("obfs", "obfs=http;obfs-host=www.bing.com;path=/cdn"))
	if err != nil {
		t.Fatal(err)
	}
	if header := obfsPath(t, node); !strings.Contains(header, "/cdn") {
		t.Fatalf("obfs path not applied: %s", header)
	}
}

func TestSSPluginObfsDefaultsToHTTPMode(t *testing.T) {
	node, err := testClient("Shadowsocks").parseSSNodeResponse(ssConfig("obfs-local", "obfs-host=www.bing.com"))
	if err != nil {
		t.Fatal(err)
	}
	obfsPath(t, node)
}

func TestSSPluginWithoutOptionsMeansNoObfs(t *testing.T) {
	// Subscriptions only emit a plugin when plugin_opts is set as well.
	node, err := testClient("Shadowsocks").parseSSNodeResponse(ssConfig("obfs", ""))
	if err != nil {
		t.Fatal(err)
	}
	if node.Header != nil {
		t.Fatalf("no obfs expected, got %s", node.Header)
	}
}

func TestSSLegacyObfsFieldStillWorks(t *testing.T) {
	s := ssConfig("", "")
	s.Obfs = "http"
	s.ObfsSettings.Path = "legacy"
	node, err := testClient("Shadowsocks").parseSSNodeResponse(s)
	if err != nil {
		t.Fatal(err)
	}
	if header := obfsPath(t, node); !strings.Contains(header, "/legacy") {
		t.Fatalf("legacy obfs path not applied: %s", header)
	}
}

func TestSSUnsupportedPluginsAreRejected(t *testing.T) {
	cases := map[string]*serverConfig{
		"v2ray-plugin": ssConfig("v2ray-plugin", "mode=websocket"),
		"obfs tls":     ssConfig("obfs", "obfs=tls;obfs-host=www.bing.com"),
		"legacy tls": func() *serverConfig {
			s := ssConfig("", "")
			s.Obfs = "tls"
			return s
		}(),
	}
	for name, s := range cases {
		if _, err := testClient("Shadowsocks").parseSSNodeResponse(s); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestXHTTPHostIsParsed(t *testing.T) {
	s := &serverConfig{ServerPort: 443}
	s.Network = "xhttp"
	s.NetworkSettings.Host = "cdn.example.com"
	s.NetworkSettings.Path = "/x"
	node, err := testClient("Vless").parseV2rayNodeResponse(s)
	if err != nil {
		t.Fatal(err)
	}
	if node.Host != "cdn.example.com" {
		t.Fatalf("xhttp host = %q, want cdn.example.com", node.Host)
	}
}

func TestOnlyBlockRoutesBecomeRules(t *testing.T) {
	c := testClient("Vless")
	s := &serverConfig{ServerPort: 443, Routes: []route{
		{Id: 1, Match: []string{"ads\\.example"}, Action: "block"},
		{Id: 2, Match: []string{"geosite:cn"}, Action: "direct"},
		{Id: 3, Match: []string{"example.org"}, Action: "dns", ActionValue: "1.1.1.1"},
	}}
	c.resp.Store(s)

	rules, err := c.GetNodeRule()
	if err != nil {
		t.Fatal(err)
	}
	if len(*rules) != 1 {
		t.Fatalf("got %d rules, want only the block route", len(*rules))
	}
	if got, _ := c.ignoredRoutes.Load().(string); got != "2:direct,3:dns" {
		t.Fatalf("ignored routes = %q", got)
	}
}

func TestXboardReportCapabilities(t *testing.T) {
	var client any = testClient("Vless")
	if _, ok := client.(api.NodeStatusReporter); !ok {
		t.Error("node status is reported to UniProxy/status")
	}
	if _, ok := client.(api.OnlineUserReporter); !ok {
		t.Error("online users are reported to UniProxy/alive")
	}
	if _, ok := client.(api.IllegalReporter); ok {
		t.Error("Xboard has no audit endpoint; the adapter must not implement IllegalReporter")
	}
	if _, ok := client.(api.TrafficReporter); !ok {
		t.Error("traffic reporting must still be implemented")
	}
}

func TestPanelFlowIsAuthoritative(t *testing.T) {
	cases := map[string]string{"": "", "none": "", "None": "", "xtls-rprx-vision": "xtls-rprx-vision"}
	for panelFlow, want := range cases {
		s := &serverConfig{ServerPort: 443}
		s.Network = "tcp"
		s.VlessFlow = panelFlow
		node, err := testClient("Vless").parseV2rayNodeResponse(s)
		if err != nil {
			t.Fatal(err)
		}
		if node.VlessFlow != want {
			t.Errorf("panel flow %q: node flow = %q, want %q", panelFlow, node.VlessFlow, want)
		}
	}
}

func xhttpConfig(extra string) *serverConfig {
	s := &serverConfig{ServerPort: 443}
	s.Network = "xhttp"
	s.NetworkSettings.Host = "cdn.example.com"
	s.NetworkSettings.Path = "/x"
	s.NetworkSettings.Mode = "stream-up"
	if extra != "" {
		s.NetworkSettings.Extra = json.RawMessage(extra)
	}
	return s
}

func TestXHTTPModeAndExtraReachTheNode(t *testing.T) {
	node, err := testClient("Vless").parseV2rayNodeResponse(xhttpConfig(
		`{"xPaddingBytes":"200-800","scMaxEachPostBytes":500000,"xmux":{"maxConcurrency":"16-32"},"downloadSettings":{"address":"","port":443}}`))
	if err != nil {
		t.Fatal(err)
	}
	if node.XHTTPMode != "stream-up" {
		t.Fatalf("mode = %q, want stream-up", node.XHTTPMode)
	}
	var extra map[string]any
	if err := json.Unmarshal(node.XHTTPExtra, &extra); err != nil {
		t.Fatalf("extra is not an object: %v", err)
	}
	if extra["xPaddingBytes"] != "200-800" || extra["scMaxEachPostBytes"] != float64(500000) {
		t.Fatalf("server settings lost: %s", node.XHTTPExtra)
	}
	for _, key := range []string{"xmux", "downloadSettings"} {
		if _, ok := extra[key]; ok {
			t.Errorf("client-only key %q must be removed: %s", key, node.XHTTPExtra)
		}
	}
}

func TestXHTTPEmptyExtraMeansNone(t *testing.T) {
	for _, extra := range []string{"", "null", "[]", "{}", `{"downloadSettings":{}}`} {
		node, err := testClient("Vless").parseV2rayNodeResponse(xhttpConfig(extra))
		if err != nil {
			t.Fatalf("extra %q: %v", extra, err)
		}
		if node.XHTTPExtra != nil {
			t.Errorf("extra %q: want none, got %s", extra, node.XHTTPExtra)
		}
	}
}

func TestXHTTPExtraMustBeAnObject(t *testing.T) {
	if _, err := testClient("Vless").parseV2rayNodeResponse(xhttpConfig(`["a"]`)); err == nil {
		t.Fatal("a non-object extra must be rejected")
	}
}

func TestXHTTPExtraIsStableAcrossPulls(t *testing.T) {
	a, err := serverXHTTPExtra(json.RawMessage(`{"b":1,"a":2}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := serverXHTTPExtra(json.RawMessage(`{ "a": 2, "b": 1 }`))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("equivalent extras encode differently: %s vs %s", a, b)
	}
}

func TestRealityDest(t *testing.T) {
	cases := []struct {
		name, dest, sni, port, want string
	}{
		{"fallback to sni and port", "", "www.example.com", "8443", "www.example.com:8443"},
		{"empty server_port means 443", "", "www.example.com", "", "www.example.com:443"},
		{"bare host gets the port", "origin.example.com", "www.example.com", "8443", "origin.example.com:8443"},
		{"host:port kept as is", "origin.example.com:9443", "www.example.com", "443", "origin.example.com:9443"},
		{"ipv6 with port kept", "[2001:db8::1]:443", "www.example.com", "443", "[2001:db8::1]:443"},
		{"bare ipv6 gets the port", "[2001:db8::1]", "www.example.com", "8443", "[2001:db8::1]:8443"},
		{"local port kept", "8443", "www.example.com", "443", "8443"},
		{"unix socket kept", "/dev/shm/h1.sock", "www.example.com", "443", "/dev/shm/h1.sock"},
		{"abstract socket kept", "@h1", "www.example.com", "443", "@h1"},
		{"nothing to build from", "", "", "443", ""},
	}
	for _, tc := range cases {
		if got := realityDest(tc.dest, tc.sni, tc.port); got != tc.want {
			t.Errorf("%s: realityDest(%q, %q, %q) = %q, want %q", tc.name, tc.dest, tc.sni, tc.port, got, tc.want)
		}
	}
}

func TestPanelRealityDestAndXverReachTheNode(t *testing.T) {
	var s serverConfig
	if err := json.Unmarshal([]byte(`{"server_port":443,"network":"tcp","tls":2,"flow":"xtls-rprx-vision",
		"tls_settings":{"server_name":"www.example.com","server_port":"443","dest":"origin.example.com:9443","xver":1,
		"private_key":"k","short_id":"ab"}}`), &s); err != nil {
		t.Fatal(err)
	}
	node, err := testClient("Vless").parseV2rayNodeResponse(&s)
	if err != nil {
		t.Fatal(err)
	}
	if !node.EnableREALITY || node.REALITYConfig == nil {
		t.Fatal("REALITY not enabled")
	}
	if node.REALITYConfig.Dest != "origin.example.com:9443" {
		t.Errorf("Dest = %q", node.REALITYConfig.Dest)
	}
	if node.REALITYConfig.ProxyProtocolVer != 1 {
		t.Errorf("xver = %d, want 1", node.REALITYConfig.ProxyProtocolVer)
	}
}

func TestPanelRealityWithoutDestStillWorks(t *testing.T) {
	// Existing Xboard nodes: no dest, xver 0, server_port null.
	var s serverConfig
	if err := json.Unmarshal([]byte(`{"server_port":443,"network":"tcp","tls":2,
		"tls_settings":{"server_name":"www.example.com","server_port":null,"dest":null,"xver":0,"private_key":"k","short_id":"ab"}}`), &s); err != nil {
		t.Fatal(err)
	}
	node, err := testClient("Vless").parseV2rayNodeResponse(&s)
	if err != nil {
		t.Fatal(err)
	}
	if node.REALITYConfig.Dest != "www.example.com:443" {
		t.Errorf("Dest = %q, want www.example.com:443", node.REALITYConfig.Dest)
	}
}

func TestPanelCertKeepsOnlyNonSecretFields(t *testing.T) {
	var s serverConfig
	if err := json.Unmarshal([]byte(`{"server_port":443,"cert_config":{"cert_mode":"DNS","domain":" node.example.com ",
		"email":"ops@example.com","dns_provider":"cloudflare","dns_env":{"CF_DNS_API_TOKEN":"secret-token"},
		"cert_content":"-----BEGIN CERTIFICATE-----","key_content":"-----BEGIN PRIVATE KEY-----"}}`), &s); err != nil {
		t.Fatal(err)
	}
	cert := s.panelCert()
	if cert == nil || cert.Mode != "dns" || cert.Domain != "node.example.com" || cert.Provider != "cloudflare" {
		t.Fatalf("panel cert = %+v", cert)
	}
	if encoded, _ := json.Marshal(cert); strings.Contains(string(encoded), "secret-token") || strings.Contains(string(encoded), "BEGIN") {
		t.Fatalf("secrets kept: %s", encoded)
	}
}

func TestPanelCertNoneMeansNil(t *testing.T) {
	for _, body := range []string{`{}`, `{"cert_config":null}`, `{"cert_config":{"cert_mode":"none"}}`} {
		var s serverConfig
		if err := json.Unmarshal([]byte(body), &s); err != nil {
			t.Fatal(err)
		}
		if cert := s.panelCert(); cert != nil {
			t.Errorf("%s: want nil, got %+v", body, cert)
		}
	}
}

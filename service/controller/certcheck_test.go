package controller

import (
	"strings"
	"testing"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/common/mylego"
)

func tlsNode() *api.NodeInfo {
	return &api.NodeInfo{NodeType: "Vless", NodeID: 1, Port: 443, TransportProtocol: "tcp", EnableTLS: true}
}

func joined(warnings []string) string { return strings.Join(warnings, " | ") }

func TestCertWarningsIgnoreNodesWithoutCertificates(t *testing.T) {
	plain := tlsNode()
	plain.EnableTLS = false
	reality := tlsNode()
	reality.EnableREALITY = true
	panel := &api.PanelCertConfig{Mode: "http", Domain: "a.example.com"}

	for name, node := range map[string]*api.NodeInfo{"no TLS": plain, "panel REALITY": reality, "no node": nil} {
		if w := certConfigWarnings(node, nil, false, panel); len(w) != 0 {
			t.Errorf("%s: unexpected warnings: %s", name, joined(w))
		}
	}
	if w := certConfigWarnings(tlsNode(), nil, true, panel); len(w) != 0 {
		t.Errorf("local REALITY: unexpected warnings: %s", joined(w))
	}
}

func TestCertWarningsFlagATLSNodeWithoutLocalCert(t *testing.T) {
	for name, local := range map[string]*mylego.CertConfig{"missing": nil, "none": {CertMode: "none"}} {
		w := certConfigWarnings(tlsNode(), local, false, nil)
		if len(w) != 1 || !strings.Contains(w[0], "WITHOUT TLS") {
			t.Errorf("%s: want the plaintext warning, got %s", name, joined(w))
		}
	}
	w := certConfigWarnings(tlsNode(), nil, false, &api.PanelCertConfig{Mode: "dns"})
	if len(w) != 2 || !strings.Contains(joined(w), "not applied by XrayR") {
		t.Errorf("panel cert without a local one: got %s", joined(w))
	}
}

func TestCertWarningsMatchingConfigIsQuiet(t *testing.T) {
	local := &mylego.CertConfig{CertMode: "dns", CertDomain: "Node.Example.com", Provider: "cloudflare"}
	panel := &api.PanelCertConfig{Mode: "dns", Domain: "node.example.com", Provider: "Cloudflare"}
	if w := certConfigWarnings(tlsNode(), local, false, panel); len(w) != 0 {
		t.Errorf("matching config warned: %s", joined(w))
	}
	if w := certConfigWarnings(tlsNode(), local, false, nil); len(w) != 0 {
		t.Errorf("no panel cert_config warned: %s", joined(w))
	}
}

func TestCertWarningsReportEachMismatch(t *testing.T) {
	local := &mylego.CertConfig{CertMode: "dns", CertDomain: "old.example.com", Provider: "alidns"}

	w := certConfigWarnings(tlsNode(), local, false, &api.PanelCertConfig{Mode: "http", Domain: "new.example.com"})
	if len(w) != 2 || !strings.Contains(joined(w), "cert mode") || !strings.Contains(joined(w), "cert domain") {
		t.Errorf("mode and domain mismatch: got %s", joined(w))
	}

	w = certConfigWarnings(tlsNode(), local, false, &api.PanelCertConfig{Mode: "dns", Domain: "old.example.com", Provider: "cloudflare"})
	if len(w) != 1 || !strings.Contains(w[0], "DNS provider") {
		t.Errorf("provider mismatch: got %s", joined(w))
	}

	for _, mode := range []string{"self", "content"} {
		w = certConfigWarnings(tlsNode(), local, false, &api.PanelCertConfig{Mode: mode})
		if len(w) != 1 || !strings.Contains(w[0], "not supported by XrayR") {
			t.Errorf("mode %s: got %s", mode, joined(w))
		}
	}
}

// certStubClient is a panel that also describes a certificate config.
type certStubClient struct {
	stubClient
	cert *api.PanelCertConfig
}

func (c *certStubClient) PanelCertConfig() *api.PanelCertConfig { return c.cert }

func TestCheckCertConfigRemembersWhatItReported(t *testing.T) {
	silenceLogs(t)
	users := []api.UserInfo{}
	c := newMonitorTestController(&users)
	node := tlsNode()
	c.publishState(node, &users, "tag")
	client := &certStubClient{stubClient: stubClient{node: node, users: &users}, cert: &api.PanelCertConfig{Mode: "http"}}
	c.apiClient = client
	c.config.CertConfig = &mylego.CertConfig{CertMode: "dns"}

	c.checkCertConfig()
	first, _ := c.certWarnings.Load().(string)
	if !strings.Contains(first, "cert mode") {
		t.Fatalf("mismatch not recorded: %q", first)
	}

	client.cert = &api.PanelCertConfig{Mode: "dns"}
	c.checkCertConfig()
	if cleared, _ := c.certWarnings.Load().(string); cleared != "" {
		t.Fatalf("warnings not cleared after the mismatch was fixed: %q", cleared)
	}
}

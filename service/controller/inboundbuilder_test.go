package controller_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/common/mylego"
	. "github.com/XrayR-project/XrayR/service/controller"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/transport/internet/reality"
)

func TestBuildV2ray(t *testing.T) {
	nodeInfo := &api.NodeInfo{
		NodeType:          "V2ray",
		NodeID:            1,
		Port:              1145,
		SpeedLimit:        0,
		AlterID:           2,
		TransportProtocol: "ws",
		Host:              "test.test.tk",
		Path:              "v2ray",
		EnableTLS:         false,
	}
	certConfig := &mylego.CertConfig{
		CertMode:   "http",
		CertDomain: "test.test.tk",
		Provider:   "alidns",
		Email:      "test@gmail.com",
	}
	config := &Config{
		CertConfig: certConfig,
	}
	_, err := InboundBuilder(config, nodeInfo, "test_tag")
	if err != nil {
		t.Error(err)
	}
}

func TestBuildTrojan(t *testing.T) {
	nodeInfo := &api.NodeInfo{
		NodeType:          "Trojan",
		NodeID:            1,
		Port:              1145,
		SpeedLimit:        0,
		AlterID:           2,
		TransportProtocol: "tcp",
		Host:              "trojan.test.tk",
		Path:              "v2ray",
		EnableTLS:         false,
	}
	DNSEnv := make(map[string]string)
	DNSEnv["ALICLOUD_ACCESS_KEY"] = "aaa"
	DNSEnv["ALICLOUD_SECRET_KEY"] = "bbb"
	certConfig := &mylego.CertConfig{
		CertMode:   "dns",
		CertDomain: "trojan.test.tk",
		Provider:   "alidns",
		Email:      "test@gmail.com",
		DNSEnv:     DNSEnv,
	}
	config := &Config{
		CertConfig: certConfig,
	}
	_, err := InboundBuilder(config, nodeInfo, "test_tag")
	if err != nil {
		t.Error(err)
	}
}

func TestBuildSS(t *testing.T) {
	nodeInfo := &api.NodeInfo{
		NodeType:          "Shadowsocks",
		NodeID:            1,
		Port:              1145,
		SpeedLimit:        0,
		AlterID:           2,
		CypherMethod:      "aes-256-gcm",
		TransportProtocol: "tcp",
		Host:              "test.test.tk",
		Path:              "v2ray",
		EnableTLS:         false,
	}
	DNSEnv := make(map[string]string)
	DNSEnv["ALICLOUD_ACCESS_KEY"] = "aaa"
	DNSEnv["ALICLOUD_SECRET_KEY"] = "bbb"
	certConfig := &mylego.CertConfig{
		CertMode:   "dns",
		CertDomain: "trojan.test.tk",
		Provider:   "alidns",
		Email:      "test@me.com",
		DNSEnv:     DNSEnv,
	}
	config := &Config{
		CertConfig: certConfig,
	}
	_, err := InboundBuilder(config, nodeInfo, "test_tag")
	if err != nil {
		t.Error(err)
	}
}

// ssNode builds a Shadowsocks node with the given cipher.
func ssNode(cipher string) *api.NodeInfo {
	return &api.NodeInfo{
		NodeType:          "Shadowsocks",
		NodeID:            1,
		Port:              1145,
		CypherMethod:      cipher,
		ServerKey:         "R3Xq0kQmYb2vN8pH4tZ6wL1sJ5jF9cA7gD3eV8bM2K4=",
		TransportProtocol: "tcp",
	}
}

// TestBuildSS2022AcceptsTheAESMethods covers the only ciphers xray-core allows in
// SS-2022 multi-user mode.
func TestBuildSS2022AcceptsTheAESMethods(t *testing.T) {
	for _, cipher := range []string{"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm"} {
		if _, err := InboundBuilder(&Config{}, ssNode(cipher), "test_tag"); err != nil {
			t.Errorf("cipher %s: unexpected error: %v", cipher, err)
		}
	}
}

// TestBuildSS2022RejectsChacha20 pins the constraint xray-core enforces only when
// it builds the inbound. XrayR always runs SS-2022 in multi-user mode, because it
// adds and removes one account per panel user and only the multi-user inbound
// implements proxy.UserManager. That mode rejects every method that is not
// blake3-aes-*-gcm. Without the check here the node died with an opaque
// "failed to build inbound handler for protocol shadowsocks" that named neither
// the cipher nor the replacement.
func TestBuildSS2022RejectsChacha20(t *testing.T) {
	_, err := InboundBuilder(&Config{}, ssNode("2022-blake3-chacha20-poly1305"), "test_tag")
	if err == nil {
		t.Fatal("expected an error for a non-AES SS-2022 cipher")
	}
	for _, want := range []string{"2022-blake3-chacha20-poly1305", "blake3-aes", "2022-blake3-aes-128-gcm"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message %q does not mention %q", err.Error(), want)
		}
	}
}

// TestBuildSSAllowsEveryNon2022Cipher checks the new guard stays out of the
// classic AEAD path, where chacha20-poly1305 is perfectly valid.
func TestBuildSSAllowsEveryNon2022Cipher(t *testing.T) {
	for _, cipher := range []string{"aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-poly1305"} {
		if _, err := InboundBuilder(&Config{}, ssNode(cipher), "test_tag"); err != nil {
			t.Errorf("cipher %s: unexpected error: %v", cipher, err)
		}
	}
}

// testRealityKey is a syntactically valid x25519 private key; xray-core rejects
// anything that does not decode to 32 bytes.
const testRealityKey = "INqp4ZipJR0DMNSWgaeyfeMDugZ7OFhcRSbQQGhTPU4"

// realityNode is the node a panel describes for VLESS + REALITY. Every REALITY
// parameter lives in the panel's node configuration, not in the XrayR config file.
func realityNode() *api.NodeInfo {
	return &api.NodeInfo{
		NodeType:          "Vless",
		NodeID:            7,
		Port:              443,
		TransportProtocol: "tcp",
		EnableTLS:         true,
		EnableREALITY:     true,
		EnableVless:       true,
		VlessFlow:         "xtls-rprx-vision",
		REALITYConfig: &api.REALITYConfig{
			Dest:        "www.amazon.com:443",
			ServerNames: []string{"www.amazon.com"},
			PrivateKey:  testRealityKey,
			ShortIds:    []string{"0123456789abcdef"},
		},
	}
}

// realitySettingsOf returns the REALITY settings xray-core would receive, or nil
// when the inbound was built without REALITY.
func realitySettingsOf(t *testing.T, config *Config, node *api.NodeInfo) *reality.Config {
	t.Helper()
	inbound, err := InboundBuilder(config, node, "test_tag")
	if err != nil {
		t.Fatalf("InboundBuilder: %v", err)
	}
	instance, err := inbound.ReceiverSettings.GetInstance()
	if err != nil {
		t.Fatalf("decode receiver settings: %v", err)
	}
	receiver, ok := instance.(*proxyman.ReceiverConfig)
	if !ok {
		t.Fatalf("unexpected receiver type %T", instance)
	}
	for _, setting := range receiver.StreamSettings.GetSecuritySettings() {
		decoded, err := setting.GetInstance()
		if err != nil {
			t.Fatalf("decode security settings: %v", err)
		}
		if settings, ok := decoded.(*reality.Config); ok {
			return settings
		}
	}
	return nil
}

// TestBuildRealityUsesPanelSettings is the ordinary Xboard setup: the node is
// REALITY in the panel and the XrayR config file says nothing about REALITY. The
// panel's parameters must be used, otherwise the node listens as plain VLESS
// while every client tries a REALITY handshake.
func TestBuildRealityUsesPanelSettings(t *testing.T) {
	settings := realitySettingsOf(t, &Config{}, realityNode())
	if settings == nil {
		t.Fatal("inbound was built without REALITY; the panel's REALITY settings were ignored")
	}
	if settings.Dest != "www.amazon.com:443" {
		t.Errorf("Dest = %q, want the panel's %q", settings.Dest, "www.amazon.com:443")
	}
	if len(settings.ServerNames) != 1 || settings.ServerNames[0] != "www.amazon.com" {
		t.Errorf("ServerNames = %v, want [www.amazon.com]", settings.ServerNames)
	}
	if string(settings.PrivateKey) != decodedTestRealityKey(t) {
		t.Errorf("PrivateKey was not taken from the panel")
	}
	if len(settings.ShortIds) != 1 {
		t.Errorf("ShortIds = %v, want one entry from the panel", settings.ShortIds)
	}
}

// TestBuildRealityPanelWinsOverLocalSettings pins the precedence: when the panel
// describes the node as REALITY, its parameters are the ones the panel hands to
// clients, so they must be the ones the server uses. A stale local block must not
// put the server and the clients on different realities.
func TestBuildRealityPanelWinsOverLocalSettings(t *testing.T) {
	config := &Config{
		EnableREALITY: true,
		REALITYConfigs: &REALITYConfig{
			Dest:        "www.microsoft.com:443",
			ServerNames: []string{"www.microsoft.com"},
			PrivateKey:  testRealityKey,
			ShortIds:    []string{""},
		},
	}
	settings := realitySettingsOf(t, config, realityNode())
	if settings == nil {
		t.Fatal("inbound was built without REALITY")
	}
	if settings.Dest != "www.amazon.com:443" {
		t.Errorf("Dest = %q, want the panel's %q", settings.Dest, "www.amazon.com:443")
	}
	if len(settings.ServerNames) != 1 || settings.ServerNames[0] != "www.amazon.com" {
		t.Errorf("ServerNames = %v, want the panel's [www.amazon.com]", settings.ServerNames)
	}
}

// TestBuildRealityFallsBackToLocalSettings covers a node whose panel does not
// describe REALITY at all: the local block is still usable, which is the only
// reason the field survived.
func TestBuildRealityFallsBackToLocalSettings(t *testing.T) {
	node := realityNode()
	node.EnableREALITY = false
	node.REALITYConfig = nil

	config := &Config{
		EnableREALITY: true,
		REALITYConfigs: &REALITYConfig{
			Dest:        "www.microsoft.com:443",
			ServerNames: []string{"www.microsoft.com"},
			PrivateKey:  testRealityKey,
			ShortIds:    []string{""},
		},
	}
	settings := realitySettingsOf(t, config, node)
	if settings == nil {
		t.Fatal("inbound was built without REALITY")
	}
	if settings.Dest != "www.microsoft.com:443" {
		t.Errorf("Dest = %q, want the locally configured %q", settings.Dest, "www.microsoft.com:443")
	}
}

// TestBuildRealityFallbackCanBeDisabled checks DisableLocalREALITYConfig still
// suppresses the local fallback for a node the panel does not describe as REALITY.
func TestBuildRealityFallbackCanBeDisabled(t *testing.T) {
	node := realityNode()
	node.EnableREALITY = false
	node.REALITYConfig = nil

	config := &Config{
		DisableLocalREALITYConfig: true,
		EnableREALITY:             true,
		REALITYConfigs: &REALITYConfig{
			Dest:        "www.microsoft.com:443",
			ServerNames: []string{"www.microsoft.com"},
			PrivateKey:  testRealityKey,
			ShortIds:    []string{""},
		},
	}
	if settings := realitySettingsOf(t, config, node); settings != nil {
		t.Errorf("REALITY was built from local settings despite DisableLocalREALITYConfig")
	}
}

// TestBuildRealityFromPanelWithoutLocalConfig covers DisableLocalREALITYConfig
// with no REALITYConfigs block. That used to dereference a nil pointer and take
// the process down before the node ever started.
func TestBuildRealityFromPanelWithoutLocalConfig(t *testing.T) {
	settings := realitySettingsOf(t, &Config{DisableLocalREALITYConfig: true}, realityNode())
	if settings == nil {
		t.Fatal("inbound was built without REALITY")
	}
	if settings.Dest != "www.amazon.com:443" {
		t.Errorf("Dest = %q, want the panel's %q", settings.Dest, "www.amazon.com:443")
	}
}

// decodedTestRealityKey returns the raw 32 bytes xray-core ends up with: the
// config builder base64-decodes the private key without padding, so the built
// settings hold the decoded key rather than the string from the config file.
func decodedTestRealityKey(t *testing.T) string {
	t.Helper()
	key, err := base64.RawStdEncoding.DecodeString(testRealityKey)
	if err != nil {
		t.Fatalf("decode test key: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("test key decodes to %d bytes, want 32", len(key))
	}
	return string(key)
}

func xhttpNode(mode, extra string) *api.NodeInfo {
	node := &api.NodeInfo{
		NodeType:          "Vless",
		NodeID:            1,
		Port:              1146,
		TransportProtocol: "xhttp",
		Host:              "cdn.example.com",
		Path:              "/x",
		XHTTPMode:         mode,
	}
	if extra != "" {
		node.XHTTPExtra = []byte(extra)
	}
	return node
}

func TestBuildXHTTPWithModeAndExtra(t *testing.T) {
	_, err := InboundBuilder(&Config{}, xhttpNode("stream-up", `{"xPaddingBytes":"200-800","scMaxEachPostBytes":500000}`), "xhttp_tag")
	if err != nil {
		t.Fatalf("valid xhttp settings rejected: %v", err)
	}
}

func TestBuildXHTTPRejectsAnUnknownMode(t *testing.T) {
	if _, err := InboundBuilder(&Config{}, xhttpNode("bogus", ""), "xhttp_tag"); err == nil {
		t.Fatal("an unknown xhttp mode must fail the build, so the node keeps its previous config")
	}
}

// Package controller Package generate the InboundConfig used by add inbound
package controller

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	C "github.com/sagernet/sing/common"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/common/mylego"
)

// InboundBuilder build Inbound config for different protocol
func InboundBuilder(config *Config, nodeInfo *api.NodeInfo, tag string) (*core.InboundHandlerConfig, error) {
	inboundDetourConfig := &conf.InboundDetourConfig{}
	// Build Listen IP address
	if config.ListenIP != "" {
		inboundDetourConfig.ListenOn = &conf.Address{Address: net.ParseAddress(config.ListenIP)}
	}

	// Build Port
	portList := &conf.PortList{
		Range: []conf.PortRange{{From: nodeInfo.Port, To: nodeInfo.Port}},
	}
	inboundDetourConfig.PortList = portList
	// Build Tag
	inboundDetourConfig.Tag = tag
	// SniffingConfig
	sniffingConfig := &conf.SniffingConfig{
		Enabled:      true,
		DestOverride: &conf.StringList{"http", "tls", "quic", "fakedns"},
	}
	if config.DisableSniffing {
		sniffingConfig.Enabled = false
	}
	inboundDetourConfig.SniffingConfig = sniffingConfig

	var (
		protocol      string
		streamSetting *conf.StreamConfig
		setting       json.RawMessage
	)

	var proxySetting any
	// Build Protocol and Protocol setting
	switch nodeInfo.NodeType {
	case "V2ray", "Vmess", "Vless":
		if nodeInfo.EnableVless || (nodeInfo.NodeType == "Vless" && nodeInfo.NodeType != "Vmess") {
			protocol = "vless"
			// The panel may enable VLESS Encryption and hand over the server
			// private key as the decryption string. xray-core rejects an empty
			// decryption, and rejects any decryption other than "none" together
			// with fallbacks, so resolve both constraints here.
			decryption := nodeInfo.Decryption
			if decryption == "" {
				decryption = "none"
			}
			// Enable fallback
			if config.EnableFallback {
				fallbackConfigs, err := buildVlessFallbacks(config.FallBackConfigs)
				if err == nil {
					if decryption != "none" {
						return nil, fmt.Errorf("vless decryption %q cannot be combined with fallbacks; disable EnableFallback or turn off VLESS Encryption in the panel", decryption)
					}
					proxySetting = &conf.VLessInboundConfig{
						Decryption: decryption,
						Fallbacks:  fallbackConfigs,
					}
				} else {
					return nil, err
				}
			} else {
				proxySetting = &conf.VLessInboundConfig{
					Decryption: decryption,
				}
			}
		} else {
			protocol = "vmess"
			proxySetting = &conf.VMessInboundConfig{}
		}
	case "Trojan":
		protocol = "trojan"
		// Enable fallback
		if config.EnableFallback {
			fallbackConfigs, err := buildTrojanFallbacks(config.FallBackConfigs)
			if err == nil {
				proxySetting = &conf.TrojanServerConfig{
					Fallbacks: fallbackConfigs,
				}
			} else {
				return nil, err
			}
		} else {
			proxySetting = &conf.TrojanServerConfig{}
		}
	case "Shadowsocks":
		protocol = "shadowsocks"
		cipher := strings.ToLower(nodeInfo.CypherMethod)

		proxySetting = &conf.ShadowsocksServerConfig{
			Cipher:   cipher,
			Password: nodeInfo.ServerKey, // shadowsocks2022 shareKey
		}

		proxySetting, _ := proxySetting.(*conf.ShadowsocksServerConfig)
		// shadowsocks must have a random password
		// shadowsocks2022's password == user PSK, thus should a length of string >= 32 and base64 encoder
		b := make([]byte, 32)
		rand.Read(b)
		randPasswd := hex.EncodeToString(b)
		if C.Contains(shadowaead_2022.List, cipher) {
			// The entry below is what puts xray-core into SS-2022 multi-user mode,
			// which XrayR needs because it adds and removes one account per panel
			// user and only the multi-user inbound implements proxy.UserManager.
			// That mode accepts blake3-aes-*-gcm only; anything else fails deep
			// inside xray-core with "shadowsocks 2022 (multi-user): only
			// blake3-aes-*-gcm methods are supported", which says what is wrong
			// but not what to change. Reject it here with the fix instead.
			if !strings.Contains(cipher, "aes") {
				return nil, fmt.Errorf("shadowsocks cipher %q cannot be used for a node: SS-2022 multi-user requires a blake3-aes-*-gcm method because every panel user needs their own key; use \"2022-blake3-aes-128-gcm\" or \"2022-blake3-aes-256-gcm\", or set the node to a non-2022 cipher", cipher)
			}
			proxySetting.Users = append(proxySetting.Users, &conf.ShadowsocksUserConfig{
				Password: base64.StdEncoding.EncodeToString(b),
			})
		} else {
			proxySetting.Password = randPasswd
		}

		proxySetting.NetworkList = &conf.NetworkList{"tcp", "udp"}
		proxySetting.IVCheck = true
		if config.DisableIVCheck {
			proxySetting.IVCheck = false
		}

	default:
		return nil, fmt.Errorf("unsupported node type: %s; supported: V2ray, Vmess, Vless, Trojan, Shadowsocks", nodeInfo.NodeType)
	}

	setting, err := json.Marshal(proxySetting)
	if err != nil {
		return nil, fmt.Errorf("marshal proxy %s config failed: %s", nodeInfo.NodeType, err)
	}
	inboundDetourConfig.Protocol = protocol
	inboundDetourConfig.Settings = &setting

	// Build streamSettings
	streamSetting = new(conf.StreamConfig)
	transportProtocol := conf.TransportProtocol(nodeInfo.TransportProtocol)
	networkType, err := transportProtocol.Build()
	if err != nil {
		return nil, fmt.Errorf("convert TransportProtocol failed: %s", err)
	}

	switch networkType {
	case "tcp":
		tcpSetting := &conf.TCPConfig{
			AcceptProxyProtocol: config.EnableProxyProtocol,
			HeaderConfig:        nodeInfo.Header,
		}
		streamSetting.TCPSettings = tcpSetting
	case "websocket":
		headers := make(map[string]string)
		headers["Host"] = nodeInfo.Host
		wsSettings := &conf.WebSocketConfig{
			AcceptProxyProtocol: config.EnableProxyProtocol,
			Host:                nodeInfo.Host,
			Path:                nodeInfo.Path,
			Headers:             headers,
		}
		streamSetting.WSSettings = wsSettings
	case "grpc":
		grpcSettings := &conf.GRPCConfig{
			ServiceName: nodeInfo.ServiceName,
			Authority:   nodeInfo.Authority,
		}
		streamSetting.GRPCSettings = grpcSettings
	case "httpupgrade":
		httpupgradeSettings := &conf.HttpUpgradeConfig{
			Headers:             nodeInfo.Headers,
			Path:                nodeInfo.Path,
			Host:                nodeInfo.Host,
			AcceptProxyProtocol: nodeInfo.AcceptProxyProtocol,
		}
		streamSetting.HTTPUPGRADESettings = httpupgradeSettings
	case "splithttp", "xhttp":
		// xray-core unmarshals Extra into the full XHTTP config and then puts
		// host, path and mode back from the fields here, so the panel's padding,
		// post-size and header settings reach the server.
		splithttpSetting := &conf.SplitHTTPConfig{
			Path:  nodeInfo.Path,
			Host:  nodeInfo.Host,
			Mode:  nodeInfo.XHTTPMode,
			Extra: nodeInfo.XHTTPExtra,
		}
		streamSetting.SplitHTTPSettings = splithttpSetting
	}
	streamSetting.Network = &transportProtocol

	// Build TLS and REALITY settings.
	//
	// REALITY belongs to the node configuration the panel owns: when a node is
	// REALITY the panel also supplies dest, server names, private key and short
	// ids, and those are exactly the values it hands to every client. The panel
	// therefore wins whenever it describes REALITY -- using local values there
	// would give the server one reality and the clients another. Local settings
	// remain a fallback for nodes whose panel does not describe REALITY at all.
	//
	// Two bugs came from the old precedence. "Not configured locally" was treated
	// as "no REALITY", so the default setup silently listened as plain VLESS; and
	// the panel branch read config.REALITYConfigs.Show without checking for nil.
	panelREALITY := nodeInfo.EnableREALITY && nodeInfo.REALITYConfig != nil
	localREALITY := !panelREALITY && !config.DisableLocalREALITYConfig &&
		config.EnableREALITY && config.REALITYConfigs != nil

	var isREALITY bool
	switch {
	case panelREALITY:
		isREALITY = true
		streamSetting.Security = "reality"

		r := nodeInfo.REALITYConfig
		streamSetting.REALITYSettings = &conf.REALITYConfig{
			Show:         realityShow(config),
			Dest:         []byte(`"` + r.Dest + `"`),
			Xver:         r.ProxyProtocolVer,
			ServerNames:  r.ServerNames,
			PrivateKey:   r.PrivateKey,
			MinClientVer: r.MinClientVer,
			MaxClientVer: r.MaxClientVer,
			MaxTimeDiff:  r.MaxTimeDiff,
			ShortIds:     r.ShortIds,
		}
	case localREALITY:
		isREALITY = true
		streamSetting.Security = "reality"

		r := config.REALITYConfigs
		streamSetting.REALITYSettings = &conf.REALITYConfig{
			Show:         r.Show,
			Dest:         []byte(`"` + r.Dest + `"`),
			Xver:         r.ProxyProtocolVer,
			ServerNames:  r.ServerNames,
			PrivateKey:   r.PrivateKey,
			MinClientVer: r.MinClientVer,
			MaxClientVer: r.MaxClientVer,
			MaxTimeDiff:  r.MaxTimeDiff,
			ShortIds:     r.ShortIds,
		}
	}

	if !isREALITY && nodeInfo.EnableTLS && config.CertConfig != nil && config.CertConfig.CertMode != "none" {
		streamSetting.Security = "tls"
		certFile, keyFile, err := getCertFile(config.CertConfig)
		if err != nil {
			return nil, err
		}
		tlsSettings := &conf.TLSConfig{
			RejectUnknownSNI: config.CertConfig.RejectUnknownSni,
		}
		tlsSettings.Certs = append(tlsSettings.Certs, &conf.TLSCertConfig{CertFile: certFile, KeyFile: keyFile, OcspStapling: 3600})
		streamSetting.TLSSettings = tlsSettings
	}

	// Support ProxyProtocol for any transport protocol
	if networkType != "tcp" && networkType != "ws" && config.EnableProxyProtocol {
		sockoptConfig := &conf.SocketConfig{
			AcceptProxyProtocol: config.EnableProxyProtocol,
		}
		streamSetting.SocketSettings = sockoptConfig
	}
	inboundDetourConfig.StreamSetting = streamSetting

	return inboundDetourConfig.Build()
}

func getCertFile(certConfig *mylego.CertConfig) (certFile string, keyFile string, err error) {
	switch certConfig.CertMode {
	case "file":
		if certConfig.CertFile == "" || certConfig.KeyFile == "" {
			return "", "", fmt.Errorf("cert file path or key file path not exist")
		}
		return certConfig.CertFile, certConfig.KeyFile, nil
	case "dns":
		lego, err := mylego.New(certConfig)
		if err != nil {
			return "", "", err
		}
		certPath, keyPath, err := lego.DNSCert()
		if err != nil {
			return "", "", err
		}
		return certPath, keyPath, err
	case "http", "tls":
		lego, err := mylego.New(certConfig)
		if err != nil {
			return "", "", err
		}
		certPath, keyPath, err := lego.HTTPCert()
		if err != nil {
			return "", "", err
		}
		return certPath, keyPath, err
	default:
		return "", "", fmt.Errorf("unsupported certmode: %s", certConfig.CertMode)
	}
}

func buildVlessFallbacks(fallbackConfigs []*FallBackConfig) ([]*conf.VLessInboundFallback, error) {
	if fallbackConfigs == nil {
		return nil, fmt.Errorf("you must provide FallBackConfigs")
	}

	vlessFallBacks := make([]*conf.VLessInboundFallback, len(fallbackConfigs))
	for i, c := range fallbackConfigs {

		if c.Dest == "" {
			return nil, fmt.Errorf("dest is required for fallback failed")
		}

		var dest json.RawMessage
		dest, err := json.Marshal(c.Dest)
		if err != nil {
			return nil, fmt.Errorf("marshal dest %s config failed: %s", dest, err)
		}
		vlessFallBacks[i] = &conf.VLessInboundFallback{
			Name: c.SNI,
			Alpn: c.Alpn,
			Path: c.Path,
			Dest: dest,
			Xver: c.ProxyProtocolVer,
		}
	}
	return vlessFallBacks, nil
}

func buildTrojanFallbacks(fallbackConfigs []*FallBackConfig) ([]*conf.TrojanInboundFallback, error) {
	if fallbackConfigs == nil {
		return nil, fmt.Errorf("you must provide FallBackConfigs")
	}

	trojanFallBacks := make([]*conf.TrojanInboundFallback, len(fallbackConfigs))
	for i, c := range fallbackConfigs {

		if c.Dest == "" {
			return nil, fmt.Errorf("dest is required for fallback failed")
		}

		var dest json.RawMessage
		dest, err := json.Marshal(c.Dest)
		if err != nil {
			return nil, fmt.Errorf("marshal dest %s config failed: %s", dest, err)
		}
		trojanFallBacks[i] = &conf.TrojanInboundFallback{
			Name: c.SNI,
			Alpn: c.Alpn,
			Path: c.Path,
			Dest: dest,
			Xver: c.ProxyProtocolVer,
		}
	}
	return trojanFallBacks, nil
}

// realityShow reports whether xray-core should log REALITY debugging detail.
// It is the only field taken from the local settings when the panel supplies the
// REALITY parameters, so ControllerConfig.REALITYConfigs may legitimately be nil.
func realityShow(config *Config) bool {
	if config.REALITYConfigs == nil {
		return false
	}
	return config.REALITYConfigs.Show
}

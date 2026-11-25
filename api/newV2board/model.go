package newV2board

import (
	"encoding/json"
)

type serverConfig struct {
	shadowsocks
	v2ray
	trojan

	ServerPort int `json:"server_port"`
	BaseConfig struct {
		PushInterval int `json:"push_interval"`
		PullInterval int `json:"pull_interval"`
	} `json:"base_config"`
	Routes []route `json:"routes"`
	// CertConfig is Xboard's per-node certificate setup (sent only when its
	// mode is not "none"). Secrets in it (dns_env, cert_content, key_content)
	// are deliberately not decoded.
	CertConfig *struct {
		CertMode    string `json:"cert_mode"`
		Domain      string `json:"domain"`
		Email       string `json:"email"`
		DNSProvider string `json:"dns_provider"`
	} `json:"cert_config"`
}

type shadowsocks struct {
	Cipher       string `json:"cipher"`
	Obfs         string `json:"obfs"`
	ObfsSettings struct {
		Path string `json:"path"`
		Host string `json:"host"`
	} `json:"obfs_settings"`
	ServerKey string `json:"server_key"`
	// Plugin and PluginOpts are the SIP003 settings the subscriptions hand to
	// clients (plugin=obfs, plugin_opts=obfs=http;obfs-host=...).
	Plugin     string `json:"plugin"`
	PluginOpts string `json:"plugin_opts"`
}

type v2ray struct {
	Network              string          `json:"network"`
	NetworkSettings      networkSettings `json:"networkSettings"`
	VlessNetworkSettings networkSettings `json:"network_settings"`
	VlessFlow            string          `json:"flow"`
	Decryption           string          `json:"decryption"`
	VlessTlsSettings     struct {
		ServerPort string `json:"server_port"`
		Dest       string `json:"dest"`
		Xver       uint64 `json:"xver"`
		Sni        string `json:"server_name"`
		PrivateKey string `json:"private_key"`
		ShortId    string `json:"short_id"`
	} `json:"tls_settings"`
	Tls int `json:"tls"`
}

// networkSettings is the transport block of a node. Mode and Extra are XHTTP
// only (Xboard's network_settings.mode / network_settings.extra).
type networkSettings struct {
	Path        string           `json:"path"`
	Host        string           `json:"host"`
	Headers     *json.RawMessage `json:"headers"`
	ServiceName string           `json:"serviceName"`
	Header      *json.RawMessage `json:"header"`
	Mode        string           `json:"mode"`
	Extra       json.RawMessage  `json:"extra"`
}

type trojan struct {
	Host       string `json:"host"`
	ServerName string `json:"server_name"`
}

type route struct {
	Id          int      `json:"id"`
	Match       []string `json:"match"`
	Action      string   `json:"action"`
	ActionValue string   `json:"action_value"`
}

type user struct {
	Id          int    `json:"id"`
	Uuid        string `json:"uuid"`
	SpeedLimit  int    `json:"speed_limit"`
	DeviceLimit int    `json:"device_limit"`
}

package newV2board

import (
	"encoding/json"
	"errors"
	"fmt"
	stdnet "net"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/bitly/go-simplejson"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/infra/conf"

	"github.com/XrayR-project/XrayR/api"
)

// APIClient create an api client to the panel.
type APIClient struct {
	*api.BaseClient
	EnableVless   bool
	SpeedLimit    float64
	DeviceLimit   int
	LocalRuleList []api.DetectRule
	resp          atomic.Value
	eTags         map[string]string
	pushInterval  atomic.Int64
	pullInterval  atomic.Int64
	// ignoredRoutes remembers which non-block routes were last reported, so the
	// warning is logged once per change rather than on every pull.
	ignoredRoutes atomic.Value
	// panelCert is the panel's certificate config from the last node pull.
	panelCert atomic.Pointer[api.PanelCertConfig]
	// statusUnsupported / aliveUnsupported are set when the panel answers 404
	// on the status / alive endpoint (Xboard builds without them), so the
	// report is skipped instead of failing every round.
	statusUnsupported atomic.Bool
	aliveUnsupported  atomic.Bool
}

// New create an api instance
func New(apiConfig *api.Config) *APIClient {
	base := api.NewBaseClient(apiConfig)

	var nodeType string

	if apiConfig.NodeType == "V2ray" && apiConfig.EnableVless {
		nodeType = "vless"
	} else {
		nodeType = strings.ToLower(apiConfig.NodeType)
	}
	// Create Key for each requests
	base.Client.SetQueryParams(map[string]string{
		"node_id":   strconv.Itoa(apiConfig.NodeID),
		"node_type": nodeType,
		"token":     apiConfig.Key,
	})
	// Read local rule list
	localRuleList := api.ReadLocalRuleList(apiConfig.RuleListPath)
	if apiConfig.VlessFlow != "" {
		// Xboard owns the VLESS flow (protocol_settings.flow); the node uses what
		// the panel sends so server and subscriptions always agree.
		log.WithFields(log.Fields{"panel": panelName, "node_id": apiConfig.NodeID, "VlessFlow": apiConfig.VlessFlow}).
			Info("Local VlessFlow is ignored for Xboard; the flow configured in the panel is used")
	}
	apiClient := &APIClient{
		BaseClient:    base,
		EnableVless:   apiConfig.EnableVless,
		SpeedLimit:    apiConfig.SpeedLimit,
		DeviceLimit:   apiConfig.DeviceLimit,
		LocalRuleList: localRuleList,
		eTags:         make(map[string]string),
	}
	return apiClient
}

// panelName identifies this adapter in logs and in the panel request metrics.
const panelName = "Xboard"

// parseResponse validates one panel response and records its outcome exactly
// once, so xrayr_panel_requests_total counts logical results rather than
// transport events: a 200 whose body fails to parse is reported as a failure.
func (c *APIClient) parseResponse(res *resty.Response, path string, err error) (*simplejson.Json, error) {
	if err == nil && api.IsNodeDisabledResponse(res) {
		disabled := api.NodeDisabledError(path, panelName, c.NodeID)
		api.ObservePanelError(panelName, path, disabled)
		return nil, disabled
	}
	if err := api.CheckResponse(res, path, panelName, c.NodeID, err); err != nil {
		api.ObservePanelError(panelName, path, err)
		return nil, err
	}

	rtn, err := simplejson.NewJson(res.Body())
	if err != nil {
		classified := api.ClassifyError(path, panelName, c.NodeID, res.StatusCode(), fmt.Errorf("invalid JSON response"))
		api.ObservePanelError(panelName, path, classified)
		return nil, classified
	}

	api.ObservePanelRequest(panelName, path, api.ResultSuccess)
	return rtn, nil
}

// forgetETagsIfDisabled drops every cached ETag when the panel reports the node
// disabled, so that once it is re-enabled the panel sends full payloads rather
// than a 304 against state the node has already torn down. It is only called
// from GetNodeInfo and GetUserList, which both run on the node monitor's
// goroutine, the sole writer of eTags.
func (c *APIClient) forgetETagsIfDisabled(err error) {
	if errors.Is(err, api.ErrNodeDisabled) {
		for key := range c.eTags {
			delete(c.eTags, key)
		}
	}
}

// GetNodeInfo will pull NodeInfo Config from panel
func (c *APIClient) GetNodeInfo() (nodeInfo *api.NodeInfo, err error) {
	server := new(serverConfig)
	path := "/api/v1/server/UniProxy/config"

	res, err := c.Client.R().
		SetHeader("If-None-Match", c.eTags["node"]).
		ForceContentType("application/json").
		Get(path)

	// A 304 means the cached node config is still current.
	if err := api.ApplyETag(res, c.eTags, "node", api.ErrNodeNotModified); err != nil {
		return nil, err
	}

	nodeInfoResp, err := c.parseResponse(res, path, err)
	if err != nil {
		c.forgetETagsIfDisabled(err)
		return nil, err
	}
	body, err := nodeInfoResp.Encode()
	if err != nil {
		return nil, api.ClassifyError(path, panelName, c.NodeID, res.StatusCode(), fmt.Errorf("encode node config response: %w", err))
	}
	if err := json.Unmarshal(body, server); err != nil {
		return nil, api.ClassifyError(path, panelName, c.NodeID, res.StatusCode(), fmt.Errorf("decode node config response: %w", err))
	}

	if server.ServerPort == 0 {
		return nil, errors.New("server port must > 0")
	}

	c.pushInterval.Store(int64(server.BaseConfig.PushInterval))
	c.pullInterval.Store(int64(server.BaseConfig.PullInterval))

	c.resp.Store(server)
	c.panelCert.Store(server.panelCert())

	switch c.NodeType {
	case "V2ray", "Vmess", "Vless":
		nodeInfo, err = c.parseV2rayNodeResponse(server)
	case "Trojan":
		nodeInfo, err = c.parseTrojanNodeResponse(server)
	case "Shadowsocks":
		nodeInfo, err = c.parseSSNodeResponse(server)
	default:
		return nil, fmt.Errorf("unsupported node type: %s", c.NodeType)
	}

	if err != nil {
		// Never include the response body: the node config carries secrets (the
		// SS-2022 server_key, the REALITY private key), and this error reaches the
		// logs and the diagnostics page. The wrapped error already names the field.
		return nil, fmt.Errorf("parse %s node config from panel failed: %w", c.NodeType, err)
	}

	return nodeInfo, nil
}

// GetUserList will pull user form panel
func (c *APIClient) GetUserList() (UserList *[]api.UserInfo, err error) {
	var users []*user
	path := "/api/v1/server/UniProxy/user"

	switch c.NodeType {
	case "V2ray", "Trojan", "Shadowsocks", "Vmess", "Vless":
		break
	default:
		return nil, fmt.Errorf("unsupported node type: %s", c.NodeType)
	}

	res, err := c.Client.R().
		SetHeader("If-None-Match", c.eTags["users"]).
		ForceContentType("application/json").
		Get(path)

	// A 304 means the cached user list is still current.
	if err := api.ApplyETag(res, c.eTags, "users", api.ErrUserNotModified); err != nil {
		return nil, err
	}

	usersResp, err := c.parseResponse(res, path, err)
	if err != nil {
		c.forgetETagsIfDisabled(err)
		return nil, err
	}
	// A 200 response must carry the "users" key. A missing key is a malformed
	// response, not an empty list, and must never clear the node's users.
	usersNode, ok := usersResp.CheckGet("users")
	if !ok {
		return nil, api.ClassifyError(path, panelName, c.NodeID, res.StatusCode(), errors.New("user list response has no \"users\" key"))
	}
	body, err := usersNode.Encode()
	if err != nil {
		return nil, api.ClassifyError(path, panelName, c.NodeID, res.StatusCode(), fmt.Errorf("encode user list response: %w", err))
	}
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, api.ClassifyError(path, panelName, c.NodeID, res.StatusCode(), fmt.Errorf("decode user list response: %w", err))
	}
	// An empty array is a legitimate answer (every user banned, expired or moved
	// out of the node's groups). Return an empty, non-nil list so the controller
	// can prune the node; the controller confirms it on the next pull first.
	if len(users) == 0 {
		empty := []api.UserInfo{}
		return &empty, nil
	}

	userList := make([]api.UserInfo, len(users))
	for i := 0; i < len(users); i++ {
		u := api.UserInfo{
			UID:  users[i].Id,
			UUID: users[i].Uuid,
		}

		// Support 1.7.1 speed limit
		if c.SpeedLimit > 0 {
			u.SpeedLimit = uint64(c.SpeedLimit * 1000000 / 8)
		} else {
			u.SpeedLimit = uint64(users[i].SpeedLimit * 1000000 / 8)
		}

		u.DeviceLimit = c.DeviceLimit // Local DeviceLimit wins; fall back to the panel's per-user value.
		if u.DeviceLimit == 0 {
			u.DeviceLimit = users[i].DeviceLimit
		}
		u.Email = u.UUID + "@v2board.user"
		if c.NodeType == "Shadowsocks" {
			u.Passwd = u.UUID
		}
		userList[i] = u
	}

	return &userList, nil
}

// ReportUserTraffic reports the user traffic
func (c *APIClient) ReportUserTraffic(userTraffic *[]api.UserTraffic) error {
	path := "/api/v1/server/UniProxy/push"

	// json structure: {uid1: [u, d], uid2: [u, d], uid1: [u, d], uid3: [u, d]}
	data := make(map[int][]int64, len(*userTraffic))
	for _, traffic := range *userTraffic {
		data[traffic.UID] = []int64{traffic.Upload, traffic.Download}
	}

	res, err := c.Client.R().SetBody(data).ForceContentType("application/json").Post(path)
	_, err = c.parseResponse(res, path, err)
	if err != nil {
		return err
	}

	return nil
}

// GetNodeRule implements the API interface
func (c *APIClient) GetNodeRule() (*[]api.DetectRule, error) {
	routes := c.resp.Load().(*serverConfig).Routes

	ruleList := c.LocalRuleList

	var ignored []string
	for i := range routes {
		if routes[i].Action == "block" {
			ruleList = api.AppendRule(ruleList, i, strings.Join(routes[i].Match, "|"))
		} else {
			ignored = append(ignored, fmt.Sprintf("%d:%s", routes[i].Id, routes[i].Action))
		}
	}
	c.warnIgnoredRoutes(ignored)

	return &ruleList, nil
}

// warnIgnoredRoutes logs the routes XrayR cannot enforce. Only "block" maps onto
// XrayR's audit rules; dns/direct/proxy would need a routing layer XrayR does
// not have, so they are dropped. The warning repeats only when the set changes.
func (c *APIClient) warnIgnoredRoutes(ignored []string) {
	signature := strings.Join(ignored, ",")
	previous, _ := c.ignoredRoutes.Load().(string)
	if signature == previous {
		return
	}
	c.ignoredRoutes.Store(signature)
	if signature == "" {
		return
	}
	log.WithFields(log.Fields{"panel": panelName, "node_id": c.NodeID, "routes": signature}).
		Warn("Panel routes ignored: XrayR only enforces routes with action \"block\"; dns, direct and proxy routes have no effect")
}

// ReportNodeStatus implements api.NodeStatusReporter: host load, the users the
// node serves and its synchronization state, posted to UniProxy/status.
func (c *APIClient) ReportNodeStatus(status *api.NodeStatus) error {
	if c.statusUnsupported.Load() {
		return nil
	}
	path := "/api/v1/server/UniProxy/status"
	var syncError any
	if status.SyncError != "" {
		syncError = status.SyncError
	}
	body := map[string]any{
		"cpu":        status.CPU,
		"mem":        map[string]uint64{"total": status.MemTotal, "used": status.MemUsed},
		"swap":       map[string]uint64{"total": status.SwapTotal, "used": status.SwapUsed},
		"disk":       map[string]uint64{"total": status.DiskTotal, "used": status.DiskUsed},
		"uptime":     status.Uptime,
		"users":      status.Users,
		"goroutines": status.Goroutines,
		"sync": map[string]any{
			"ok":           status.SyncError == "",
			"error":        syncError,
			"last_sync_at": status.LastSyncAt,
		},
	}
	res, err := c.Client.R().SetBody(body).ForceContentType("application/json").Post(path)
	return c.parseReport(res, path, err, &c.statusUnsupported, "node status")
}

// ReportNodeOnlineUsers implements api.OnlineUserReporter: the IPs seen per
// user during the last period, posted to UniProxy/alive as {"uid": ["ip", ...]}.
// An empty report is sent as well, so the panel's online count drops to zero.
func (c *APIClient) ReportNodeOnlineUsers(onlineUserList *[]api.OnlineUser) error {
	if c.aliveUnsupported.Load() {
		return nil
	}
	path := "/api/v1/server/UniProxy/alive"
	data := make(map[int][]string)
	if onlineUserList != nil {
		for _, online := range *onlineUserList {
			data[online.UID] = append(data[online.UID], online.IP)
		}
	}
	res, err := c.Client.R().SetBody(data).ForceContentType("application/json").Post(path)
	return c.parseReport(res, path, err, &c.aliveUnsupported, "online user")
}

// parseReport validates the response of a report. A 404 means the panel has no
// such endpoint: the report is switched off for the rest of the process with a
// single warning, rather than logging an error every period.
func (c *APIClient) parseReport(res *resty.Response, path string, err error, unsupported *atomic.Bool, name string) error {
	if err == nil && res != nil && res.StatusCode() == 404 {
		if unsupported.CompareAndSwap(false, true) {
			log.WithFields(log.Fields{"panel": panelName, "node_id": c.NodeID, "path": path}).
				Warnf("The panel has no %s endpoint (an Xboard build without it); %s reporting is disabled until XrayR restarts", name, name)
		}
		return nil
	}
	_, err = c.parseResponse(res, path, err)
	return err
}

// Xboard has no endpoint for audit hits, so this adapter deliberately does not
// implement api.IllegalReporter.

// PullInterval implements the api.IntervalsProvider interface. It returns the
// pull_interval the panel last sent, or 0 when the panel did not provide one.
func (c *APIClient) PullInterval() int {
	return int(c.pullInterval.Load())
}

// PushInterval implements the api.IntervalsProvider interface. It returns the
// push_interval the panel last sent, or 0 when the panel did not provide one.
func (c *APIClient) PushInterval() int {
	return int(c.pushInterval.Load())
}

// parseTrojanNodeResponse parse the response for the given nodeInfo format
func (c *APIClient) parseTrojanNodeResponse(s *serverConfig) (*api.NodeInfo, error) {
	// Create GeneralNodeInfo
	nodeInfo := &api.NodeInfo{
		NodeType:          c.NodeType,
		NodeID:            c.NodeID,
		Port:              uint32(s.ServerPort),
		TransportProtocol: "tcp",
		EnableTLS:         true,
		Host:              s.Host,
		ServiceName:       s.ServerName,
		NameServerConfig:  s.parseDNSConfig(),
	}
	return nodeInfo, nil
}

// parseSSNodeResponse parse the response for the given nodeInfo format
func (c *APIClient) parseSSNodeResponse(s *serverConfig) (*api.NodeInfo, error) {
	var header json.RawMessage

	mode, obfsPath, err := ssObfs(s)
	if err != nil {
		return nil, err
	}
	if mode == "http" {
		path := "/"
		if p := obfsPath; p != "" {
			if strings.HasPrefix(p, "/") {
				path = p
			} else {
				path += p
			}
		}
		h := simplejson.New()
		h.Set("type", "http")
		h.SetPath([]string{"request", "path"}, path)
		header, _ = h.Encode()
	}
	// Create GeneralNodeInfo
	return &api.NodeInfo{
		NodeType:          c.NodeType,
		NodeID:            c.NodeID,
		Port:              uint32(s.ServerPort),
		TransportProtocol: "tcp",
		CypherMethod:      s.Cipher,
		ServerKey:         s.ServerKey, // shadowsocks2022 share key
		NameServerConfig:  s.parseDNSConfig(),
		Header:            header,
	}, nil
}

// ssObfs resolves the Shadowsocks obfuscation the panel configured.
//
// Subscriptions describe obfuscation through the SIP003 plugin fields, and only
// when both plugin and plugin_opts are set, so those win and are mirrored here
// exactly; the separate obfs/obfs_settings fields are the legacy form. xray-core
// can only reproduce simple-obfs in http mode (a TCP http header). Every other
// plugin or mode is rejected, because serving without it would leave each
// client speaking a protocol the node does not.
func ssObfs(s *serverConfig) (mode, path string, err error) {
	plugin := strings.ToLower(strings.TrimSpace(s.Plugin))
	switch plugin {
	case "":
		mode, path = strings.ToLower(strings.TrimSpace(s.Obfs)), s.ObfsSettings.Path
	case "obfs", "obfs-local", "simple-obfs":
		if strings.TrimSpace(s.PluginOpts) == "" {
			// Clients get no plugin without options, so neither does the node.
			return "", "", nil
		}
		opts := parsePluginOpts(s.PluginOpts)
		mode = strings.ToLower(firstNonEmpty(opts["obfs"], opts["mode"], "http"))
		path = firstNonEmpty(opts["path"], opts["obfs-uri"])
	default:
		return "", "", fmt.Errorf("unsupported shadowsocks plugin %q: XrayR does not implement SIP003 plugins and only supports simple-obfs in http mode (plugin=obfs, plugin_opts=obfs=http;obfs-host=...)", s.Plugin)
	}
	switch mode {
	case "", "none":
		return "", "", nil
	case "http":
		return "http", path, nil
	default:
		return "", "", fmt.Errorf("unsupported shadowsocks obfs mode %q: XrayR only supports obfs=http", mode)
	}
}

// parsePluginOpts splits "k1=v1;k2=v2;flag" the way the panel's subscriptions do.
func parsePluginOpts(raw string) map[string]string {
	opts := make(map[string]string)
	for _, pair := range strings.Split(raw, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, found := strings.Cut(pair, "=")
		if !found {
			value = "true"
		}
		opts[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return opts
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// parseV2rayNodeResponse parse the response for the given nodeInfo format
func (c *APIClient) parseV2rayNodeResponse(s *serverConfig) (*api.NodeInfo, error) {
	var (
		host          string
		header        json.RawMessage
		enableTLS     bool
		enableREALITY bool
		dest          string
		xVer          uint64
		xhttpMode     string
		xhttpExtra    json.RawMessage
	)

	dest = realityDest(s.VlessTlsSettings.Dest, s.VlessTlsSettings.Sni, s.VlessTlsSettings.ServerPort)
	xVer = s.VlessTlsSettings.Xver

	realityConfig := api.REALITYConfig{
		Dest:             dest,
		ProxyProtocolVer: xVer,
		ServerNames:      []string{s.VlessTlsSettings.Sni},
		PrivateKey:       s.VlessTlsSettings.PrivateKey,
		ShortIds:         []string{s.VlessTlsSettings.ShortId},
	}

	// Xboard sends the transport settings of every node type in the top-level
	// "networkSettings" key. "network_settings" only exists for deployments of
	// other panels that keep a separate VLESS block; when the panel provides
	// nothing there, keep the parsed "networkSettings" instead of wiping the
	// transport parameters (ws path/host, grpc serviceName) with zero values.
	if c.EnableVless && vlessNetworkSettingsPresent(&s.VlessNetworkSettings) {
		s.NetworkSettings = s.VlessNetworkSettings
	}

	switch s.Network {
	case "ws":
		if s.NetworkSettings.Headers != nil {
			if httpHeader, err := s.NetworkSettings.Headers.MarshalJSON(); err != nil {
				return nil, err
			} else {
				b, _ := simplejson.NewJson(httpHeader)
				host = b.Get("Host").MustString()
			}
		}
	case "tcp":
		if s.NetworkSettings.Header != nil {
			if httpHeader, err := s.NetworkSettings.Header.MarshalJSON(); err != nil {
				return nil, err
			} else {
				header = httpHeader
			}
		}
	case "httpupgrade", "splithttp", "xhttp":
		if s.NetworkSettings.Headers != nil {
			if httpHeaders, err := s.NetworkSettings.Headers.MarshalJSON(); err != nil {
				return nil, err
			} else {
				b, _ := simplejson.NewJson(httpHeaders)
				host = b.Get("Host").MustString()
			}
		}
		if s.NetworkSettings.Host != "" {
			host = s.NetworkSettings.Host
		}
		if s.Network != "httpupgrade" {
			xhttpMode = strings.TrimSpace(s.NetworkSettings.Mode)
			extra, err := serverXHTTPExtra(s.NetworkSettings.Extra)
			if err != nil {
				return nil, err
			}
			xhttpExtra = extra
		}
	}

	switch s.Tls {
	case 0:
		enableTLS = false
		enableREALITY = false
	case 1:
		enableTLS = true
		enableREALITY = false
	case 2:
		enableTLS = true
		enableREALITY = true
	}

	// Create GeneralNodeInfo
	return &api.NodeInfo{
		NodeType:          c.NodeType,
		NodeID:            c.NodeID,
		Port:              uint32(s.ServerPort),
		AlterID:           0,
		TransportProtocol: s.Network,
		EnableTLS:         enableTLS,
		Path:              s.NetworkSettings.Path,
		Host:              host,
		EnableVless:       c.EnableVless,
		VlessFlow:         normalizeFlow(s.VlessFlow),
		Decryption:        s.Decryption,
		ServiceName:       s.NetworkSettings.ServiceName,
		Header:            header,
		XHTTPMode:         xhttpMode,
		XHTTPExtra:        xhttpExtra,
		EnableREALITY:     enableREALITY,
		REALITYConfig:     &realityConfig,
		NameServerConfig:  s.parseDNSConfig(),
	}, nil
}

// realityDest resolves the REALITY fallback target.
//
// Xboard's dest is optional and may already be complete: host:port, [IPv6]:port,
// a bare local port, or a unix socket (/path, @abstract). Only a bare host gets
// the panel's server_port appended. Without a dest the node falls back to
// server_name:server_port. An empty server_port (Xboard's default) means 443;
// the old code produced "host:" in that case, which is not a valid address.
func realityDest(dest, serverName, serverPort string) string {
	port := strings.TrimSpace(serverPort)
	if port == "" || port == "0" {
		port = "443"
	}
	dest = strings.TrimSpace(dest)
	if dest == "" {
		if serverName == "" {
			return ""
		}
		return stdnet.JoinHostPort(serverName, port)
	}
	if strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "@") {
		return dest
	}
	if _, err := strconv.ParseUint(dest, 10, 16); err == nil {
		return dest
	}
	if _, _, err := stdnet.SplitHostPort(dest); err == nil {
		return dest
	}
	return stdnet.JoinHostPort(strings.Trim(dest, "[]"), port)
}

// PanelCertConfig implements api.CertConfigProvider.
func (c *APIClient) PanelCertConfig() *api.PanelCertConfig {
	return c.panelCert.Load()
}

// panelCert extracts the non-secret certificate settings the panel sent.
func (s *serverConfig) panelCert() *api.PanelCertConfig {
	if s.CertConfig == nil {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(s.CertConfig.CertMode))
	if mode == "" || mode == "none" {
		return nil
	}
	return &api.PanelCertConfig{
		Mode:     mode,
		Domain:   strings.TrimSpace(s.CertConfig.Domain),
		Email:    strings.TrimSpace(s.CertConfig.Email),
		Provider: strings.TrimSpace(s.CertConfig.DNSProvider),
	}
}

// normalizeFlow maps the panel's "no flow" spellings to the empty flow xray-core
// expects.
func normalizeFlow(flow string) string {
	flow = strings.TrimSpace(flow)
	if strings.EqualFold(flow, "none") {
		return ""
	}
	return flow
}

// vlessNetworkSettingsPresent reports whether the panel supplied a separate
// VLESS transport block ("network_settings") with actual content.
func vlessNetworkSettingsPresent(s *networkSettings) bool {
	return s.Path != "" || s.Host != "" || s.ServiceName != "" || s.Headers != nil || s.Header != nil ||
		s.Mode != "" || len(s.Extra) > 0
}

// xhttpClientOnlyKeys are XHTTP "extra" settings that only a client uses: the
// server never dials a separate download connection and does not multiplex
// outgoing requests. Xboard's default template fills downloadSettings with an
// empty address, which the server must not try to build.
var xhttpClientOnlyKeys = []string{"downloadSettings", "xmux"}

// serverXHTTPExtra prepares the panel's XHTTP "extra" object for the server.
// Empty values (null, {}, and PHP's [] for an empty array) mean "no extra". The
// result is re-encoded with sorted keys so that an unchanged config compares
// equal between pulls.
func serverXHTTPExtra(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	switch trimmed {
	case "", "null", "[]", "{}":
		return nil, nil
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(raw, &extra); err != nil {
		return nil, fmt.Errorf("xhttp extra must be a JSON object: %w", err)
	}
	for _, key := range xhttpClientOnlyKeys {
		delete(extra, key)
	}
	if len(extra) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(extra)
	if err != nil {
		return nil, fmt.Errorf("encode xhttp extra: %w", err)
	}
	return encoded, nil
}

func (s *serverConfig) parseDNSConfig() (nameServerList []*conf.NameServerConfig) {
	for i := range s.Routes {
		if s.Routes[i].Action == "dns" {
			nameServerList = append(nameServerList, &conf.NameServerConfig{
				Address: &conf.Address{Address: net.ParseAddress(s.Routes[i].ActionValue)},
				Domains: s.Routes[i].Match,
			})
		}
	}

	return
}

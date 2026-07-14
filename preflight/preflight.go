// Package preflight performs read-only deployment checks shared by startup and doctor.
package preflight

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/XrayR-project/XrayR/api"
	xconfig "github.com/XrayR-project/XrayR/config"
	"github.com/XrayR-project/XrayR/panel"
)

type Status string

const (
	StatusOK      Status = "ok"
	StatusWarning Status = "warning"
	StatusError   Status = "error"
	StatusSkipped Status = "skipped"
)

type Result struct {
	Section    string        `json:"section"`
	Name       string        `json:"name"`
	Status     Status        `json:"status"`
	Detail     string        `json:"detail,omitempty"`
	Suggestion string        `json:"suggestion,omitempty"`
	Duration   time.Duration `json:"duration"`
}

type Options struct {
	Node    int
	Timeout time.Duration
	Remote  bool
}

// Run executes local checks and optional read-only remote panel checks.
func Run(ctx context.Context, cfg *panel.Config, options Options) []Result {
	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Second
	}
	results := validationResults(cfg)
	for index, node := range cfg.NodesConfig {
		if options.Node >= 0 && options.Node != index {
			continue
		}
		if node == nil || node.ApiConfig == nil {
			continue
		}
		prefix := fmt.Sprintf("node[%d]", index)
		results = append(results, timed("network", prefix+" panel URL", func() (Status, string, string) {
			parsed, err := url.Parse(node.ApiConfig.APIHost)
			if err != nil {
				return StatusError, "invalid URL", "fix ApiHost"
			}
			return StatusOK, parsed.Scheme + "://" + parsed.Host, ""
		}))
		results = append(results, checkNetwork(ctx, prefix, node.ApiConfig.APIHost, options.Timeout)...)
		results = append(results, checkFiles(prefix, node)...)
		if global := node.ControllerConfig.GlobalDeviceLimitConfig; global != nil && global.Enable {
			results = append(results, checkRedis(ctx, prefix, global.RedisNetwork, global.RedisAddr, global.RedisUsername, global.RedisPassword, global.RedisDB, options.Timeout))
		}
		if options.Remote {
			results = append(results, checkPanel(ctx, prefix, node, options.Timeout)...)
		}
	}
	return results
}

func validationResults(cfg *panel.Config) []Result {
	issues := xconfig.Validate(cfg)
	results := make([]Result, 0, len(issues))
	for _, issue := range issues {
		status := StatusWarning
		if issue.Severity == xconfig.SeverityError {
			status = StatusError
		}
		results = append(results, Result{Section: "configuration", Name: issue.Path, Status: status, Detail: issue.Message, Suggestion: issue.Suggestion})
	}
	if len(issues) == 0 {
		results = append(results, Result{Section: "configuration", Name: "schema", Status: StatusOK, Detail: "configuration is valid"})
	}
	return results
}

func checkNetwork(ctx context.Context, prefix, rawURL string, timeout time.Duration) []Result {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return nil
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	resolverCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	addresses, resolveErr := net.DefaultResolver.LookupHost(resolverCtx, parsed.Hostname())
	dns := Result{Section: "network", Name: prefix + " DNS", Duration: time.Since(started)}
	if resolveErr != nil {
		dns.Status = StatusError
		dns.Detail = resolveErr.Error()
		dns.Suggestion = "check DNS and panel hostname"
	} else {
		dns.Status = StatusOK
		dns.Detail = fmt.Sprintf("resolved %d address(es)", len(addresses))
	}
	results := []Result{dns}
	dialCtx, cancelDial := context.WithTimeout(ctx, timeout)
	defer cancelDial()
	started = time.Now()
	connection, dialErr := (&net.Dialer{}).DialContext(dialCtx, "tcp", net.JoinHostPort(parsed.Hostname(), port))
	tcp := Result{Section: "network", Name: prefix + " TCP", Duration: time.Since(started)}
	if dialErr != nil {
		tcp.Status = StatusError
		tcp.Detail = dialErr.Error()
		tcp.Suggestion = "check firewall, port, and panel availability"
	} else {
		tcp.Status = StatusOK
		tcp.Detail = "connection established"
		connection.Close()
	}
	results = append(results, tcp)
	if parsed.Scheme == "https" {
		results = append(results, checkTLS(ctx, prefix, parsed.Hostname(), port, timeout))
	}
	return results
}

func checkTLS(ctx context.Context, prefix, host, port string, timeout time.Duration) Result {
	started := time.Now()
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: timeout}, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	result := Result{Section: "network", Name: prefix + " TLS", Duration: time.Since(started)}
	if err != nil {
		result.Status = StatusError
		result.Detail = err.Error()
		result.Suggestion = "check certificate validity and hostname"
		return result
	}
	defer connection.Close()
	state := connection.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		result.Status = StatusError
		result.Detail = "server returned no certificate"
		return result
	}
	remaining := time.Until(state.PeerCertificates[0].NotAfter)
	result.Detail = fmt.Sprintf("certificate valid for %s", remaining.Round(time.Hour))
	if remaining < 14*24*time.Hour {
		result.Status = StatusWarning
		result.Suggestion = "renew the certificate soon"
	} else {
		result.Status = StatusOK
	}
	return result
}

func checkFiles(prefix string, node *panel.NodesConfig) []Result {
	var results []Result
	if node.ApiConfig.RuleListPath != "" {
		results = append(results, fileResult(prefix+" rule list", node.ApiConfig.RuleListPath))
	}
	if cert := node.ControllerConfig.CertConfig; cert != nil && strings.EqualFold(cert.CertMode, "file") {
		results = append(results, fileResult(prefix+" certificate", cert.CertFile), fileResult(prefix+" private key", cert.KeyFile))
		if pair, err := tls.LoadX509KeyPair(cert.CertFile, cert.KeyFile); err != nil {
			results = append(results, Result{Section: "filesystem", Name: prefix + " certificate pair", Status: StatusError, Detail: err.Error(), Suggestion: "ensure certificate and private key match"})
		} else if len(pair.Certificate) > 0 {
			if parsed, err := x509.ParseCertificate(pair.Certificate[0]); err == nil {
				results = append(results, Result{Section: "filesystem", Name: prefix + " certificate expiry", Status: StatusOK, Detail: parsed.NotAfter.Format(time.RFC3339)})
			}
		}
	}
	return results
}
func fileResult(name, path string) Result {
	started := time.Now()
	result := Result{Section: "filesystem", Name: name, Duration: time.Since(started)}
	if _, err := os.Stat(path); err != nil {
		result.Status = StatusError
		result.Detail = err.Error()
		result.Suggestion = "check path and permissions"
	} else {
		result.Status = StatusOK
		result.Detail = "readable"
	}
	return result
}
func checkRedis(ctx context.Context, prefix, network, address, username, password string, db int, timeout time.Duration) Result {
	started := time.Now()
	if network == "" {
		network = "tcp"
	}
	client := redis.NewClient(&redis.Options{Network: network, Addr: address, Username: username, Password: password, DB: db, DialTimeout: timeout})
	defer client.Close()
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := client.Ping(checkCtx).Err()
	result := Result{Section: "dependency", Name: prefix + " Redis", Duration: time.Since(started)}
	if err != nil {
		result.Status = StatusError
		result.Detail = err.Error()
		result.Suggestion = "check Redis address, credentials, and network"
	} else {
		result.Status = StatusOK
		result.Detail = "PING succeeded"
	}
	return result
}
func checkPanel(ctx context.Context, prefix string, node *panel.NodesConfig, timeout time.Duration) []Result {
	definition, err := panel.LookupPanel(node.PanelType)
	if err != nil {
		return nil
	}
	client := definition.New(node.ApiConfig)
	type outcome struct {
		name   string
		err    error
		detail string
	}
	channel := make(chan outcome, 1)
	go func() {
		info, err := client.GetNodeInfo()
		detail := ""
		if info != nil {
			detail = fmt.Sprintf("protocol=%s port=%d", info.NodeType, info.Port)
		}
		channel <- outcome{name: "node config", err: err, detail: detail}
	}()
	results := make([]Result, 0, 2)
	select {
	case result := <-channel:
		results = append(results, panelResult(prefix+" "+result.name, result.err, result.detail))
	case <-time.After(timeout):
		results = append(results, Result{Section: "panel", Name: prefix + " node config", Status: StatusError, Detail: "request timed out", Suggestion: "check Timeout and panel availability"})
	}
	channel = make(chan outcome, 1)
	go func() {
		users, err := client.GetUserList()
		detail := ""
		if users != nil {
			detail = fmt.Sprintf("%d users", len(*users))
		}
		channel <- outcome{name: "user list", err: err, detail: detail}
	}()
	select {
	case result := <-channel:
		results = append(results, panelResult(prefix+" "+result.name, result.err, result.detail))
	case <-time.After(timeout):
		results = append(results, Result{Section: "panel", Name: prefix + " user list", Status: StatusError, Detail: "request timed out", Suggestion: "check Timeout and panel availability"})
	}
	if closer, ok := client.(api.Closer); ok {
		_ = closer.Close()
	}
	return results
}
func panelResult(name string, err error, detail string) Result {
	result := Result{Section: "panel", Name: name, Detail: detail}
	if err != nil {
		result.Status = StatusError
		result.Detail = err.Error()
		result.Suggestion = "verify ApiKey, NodeID, NodeType, and panel API compatibility"
	} else {
		result.Status = StatusOK
	}
	return result
}
func timed(section, name string, fn func() (Status, string, string)) Result {
	started := time.Now()
	status, detail, suggestion := fn()
	return Result{Section: section, Name: name, Status: status, Detail: detail, Suggestion: suggestion, Duration: time.Since(started)}
}

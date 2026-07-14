package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/XrayR-project/XrayR/panel"
)

// Validate performs local checks only and returns every issue found.
func Validate(cfg *panel.Config) []Issue {
	issues := make([]Issue, 0)
	if cfg.ConfigVersion == 0 {
		issues = append(issues, Issue{Severity: SeverityInfo, Path: "ConfigVersion", Message: "unversioned legacy configuration is supported", Suggestion: "run XrayR config migrate to add ConfigVersion"})
	} else if cfg.ConfigVersion > CurrentVersion {
		issues = append(issues, Issue{Severity: SeverityError, Path: "ConfigVersion", Message: fmt.Sprintf("configuration version %d is newer than supported version %d", cfg.ConfigVersion, CurrentVersion), Suggestion: "upgrade XrayR or migrate with a compatible version"})
	}
	if cfg.LogConfig != nil {
		if !containsFold([]string{"none", "error", "warning", "info", "debug"}, cfg.LogConfig.Level) {
			issues = append(issues, Issue{Severity: SeverityError, Path: "Log.Level", Message: "unsupported log level", Suggestion: "use none, error, warning, info, or debug"})
		}
		if !containsFold([]string{"text", "json"}, cfg.LogConfig.Format) {
			issues = append(issues, Issue{Severity: SeverityError, Path: "Log.Format", Message: "unsupported log format", Suggestion: "use text or json"})
		}
	}
	if cfg.Diagnostics != nil && cfg.Diagnostics.Enable {
		if _, _, err := net.SplitHostPort(cfg.Diagnostics.Listen); err != nil {
			issues = append(issues, Issue{Severity: SeverityError, Path: "Diagnostics.Listen", Message: "invalid listen address", Suggestion: "use host:port, for example 127.0.0.1:8080"})
		}
	}
	if len(cfg.NodesConfig) == 0 {
		issues = append(issues, Issue{Severity: SeverityError, Path: "Nodes", Message: "at least one node is required", Suggestion: "add a node under Nodes"})
		return issues
	}

	identities := make(map[string]int)
	for i, node := range cfg.NodesConfig {
		prefix := fmt.Sprintf("Nodes[%d]", i)
		if node == nil {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix, Message: "node cannot be null"})
			continue
		}
		definition, err := panel.LookupPanel(node.PanelType)
		if err != nil {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".PanelType", Message: err.Error(), Suggestion: "choose a supported panel name or alias"})
		}
		if node.ApiConfig == nil {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ApiConfig", Message: "ApiConfig is required"})
			continue
		}
		apiConfig := node.ApiConfig
		if parsed, parseErr := url.ParseRequestURI(apiConfig.APIHost); parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ApiConfig.ApiHost", Message: "ApiHost must be a valid HTTP or HTTPS URL", Suggestion: "use a URL such as https://panel.example.com"})
		}
		if strings.TrimSpace(apiConfig.Key) == "" {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ApiConfig.ApiKey", Message: "ApiKey cannot be empty", Suggestion: "copy the node API key from the panel"})
		}
		if apiConfig.NodeID <= 0 {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ApiConfig.NodeID", Message: "NodeID must be greater than zero"})
		}
		if err == nil && !definition.SupportsNodeType(apiConfig.NodeType) {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ApiConfig.NodeType", Message: fmt.Sprintf("node type %q is not supported by %s", apiConfig.NodeType, definition.Name), Suggestion: "supported values: " + strings.Join(definition.NodeTypes, ", ")})
		}
		if apiConfig.Timeout < 1 {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ApiConfig.Timeout", Message: "Timeout must be at least one second"})
		}
		if apiConfig.RuleListPath != "" {
			issues = append(issues, validateRuleFile(prefix+".ApiConfig.RuleListPath", apiConfig.RuleListPath)...)
		}
		if node.ControllerConfig == nil {
			continue
		}
		controllerCfg := node.ControllerConfig
		if controllerCfg.UpdatePeriodic < 1 {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ControllerConfig.UpdatePeriodic", Message: "UpdatePeriodic must be greater than zero"})
		} else if controllerCfg.UpdatePeriodic < 30 {
			issues = append(issues, Issue{Severity: SeverityWarning, Path: prefix + ".ControllerConfig.UpdatePeriodic", Message: "update period is shorter than 30 seconds", Suggestion: "use at least 30 seconds to avoid excessive panel requests"})
		}
		if !containsFold([]string{"AsIs", "UseIP", "UseIPv4", "UseIPv6"}, controllerCfg.DNSType) {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ControllerConfig.DNSType", Message: "unsupported DNS strategy", Suggestion: "use AsIs, UseIP, UseIPv4, or UseIPv6"})
		}
		if global := controllerCfg.GlobalDeviceLimitConfig; global != nil && global.Enable {
			if strings.TrimSpace(global.RedisAddr) == "" {
				issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ControllerConfig.GlobalDeviceLimitConfig.RedisAddr", Message: "RedisAddr is required when global device limit is enabled"})
			}
			if global.Timeout <= 0 || global.Expiry <= 0 {
				issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ControllerConfig.GlobalDeviceLimitConfig", Message: "Redis Timeout and Expiry must be greater than zero"})
			}
		}
		if controllerCfg.EnableREALITY && controllerCfg.REALITYConfigs != nil && !controllerCfg.DisableLocalREALITYConfig {
			reality := controllerCfg.REALITYConfigs
			if reality.Dest == "" || len(reality.ServerNames) == 0 || reality.PrivateKey == "" {
				issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ControllerConfig.REALITYConfigs", Message: "local REALITY requires Dest, ServerNames, and PrivateKey", Suggestion: "provide the fields or enable DisableLocalREALITYConfig"})
			}
		}
		if cert := controllerCfg.CertConfig; cert != nil && strings.EqualFold(cert.CertMode, "file") {
			for field, path := range map[string]string{"CertFile": cert.CertFile, "KeyFile": cert.KeyFile} {
				if path == "" {
					issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ControllerConfig.CertConfig." + field, Message: field + " is required for file certificate mode"})
				} else if _, statErr := os.Stat(path); statErr != nil {
					issues = append(issues, Issue{Severity: SeverityError, Path: prefix + ".ControllerConfig.CertConfig." + field, Message: "file is not readable", Suggestion: statErr.Error()})
				}
			}
		}
		identity := strings.ToLower(fmt.Sprintf("%s|%s|%d|%s", definition.Name, apiConfig.APIHost, apiConfig.NodeID, apiConfig.NodeType))
		if previous, exists := identities[identity]; exists {
			issues = append(issues, Issue{Severity: SeverityError, Path: prefix, Message: fmt.Sprintf("duplicates Nodes[%d]", previous), Suggestion: "remove the duplicate node configuration"})
		} else {
			identities[identity] = i
		}
	}
	return issues
}

func validateRuleFile(path, filename string) []Issue {
	data, err := os.ReadFile(filename)
	if err != nil {
		return []Issue{{Severity: SeverityError, Path: path, Message: "rule file is not readable", Suggestion: err.Error()}}
	}
	var issues []Issue
	for line, value := range strings.Split(string(data), "\n") {
		value = strings.TrimSpace(value)
		if value == "" || strings.HasPrefix(value, "#") {
			continue
		}
		if _, err := regexp.Compile(value); err != nil {
			issues = append(issues, Issue{Severity: SeverityError, Path: fmt.Sprintf("%s:%d", path, line+1), Message: "invalid regular expression", Suggestion: err.Error()})
		}
	}
	return issues
}

func containsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

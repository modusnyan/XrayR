package redact

import (
	"net/url"
	"strings"

	"github.com/XrayR-project/XrayR/panel"
)

const Mask = "***REDACTED***"

// URL removes query parameters and user information from a URL.
func URL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		if index := strings.IndexByte(value, '?'); index >= 0 {
			return value[:index]
		}
		return value
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.User = nil
	return parsed.String()
}

// Config returns a deep-enough copy suitable for display and status endpoints.
func Config(source *panel.Config) *panel.Config {
	if source == nil {
		return nil
	}
	copyConfig := *source
	copyConfig.NodesConfig = make([]*panel.NodesConfig, len(source.NodesConfig))
	for i, sourceNode := range source.NodesConfig {
		if sourceNode == nil {
			continue
		}
		node := *sourceNode
		if sourceNode.ApiConfig != nil {
			apiConfig := *sourceNode.ApiConfig
			if apiConfig.Key != "" {
				apiConfig.Key = Mask
			}
			apiConfig.APIHost = URL(apiConfig.APIHost)
			node.ApiConfig = &apiConfig
		}
		if sourceNode.ControllerConfig != nil {
			controllerConfig := *sourceNode.ControllerConfig
			if sourceNode.ControllerConfig.GlobalDeviceLimitConfig != nil {
				global := *sourceNode.ControllerConfig.GlobalDeviceLimitConfig
				if global.RedisPassword != "" {
					global.RedisPassword = Mask
				}
				controllerConfig.GlobalDeviceLimitConfig = &global
			}
			if sourceNode.ControllerConfig.REALITYConfigs != nil {
				reality := *sourceNode.ControllerConfig.REALITYConfigs
				if reality.PrivateKey != "" {
					reality.PrivateKey = Mask
				}
				controllerConfig.REALITYConfigs = &reality
			}
			if sourceNode.ControllerConfig.CertConfig != nil {
				cert := *sourceNode.ControllerConfig.CertConfig
				if cert.DNSEnv != nil {
					cert.DNSEnv = make(map[string]string, len(cert.DNSEnv))
					for key := range sourceNode.ControllerConfig.CertConfig.DNSEnv {
						cert.DNSEnv[key] = Mask
					}
				}
				controllerConfig.CertConfig = &cert
			}
			node.ControllerConfig = &controllerConfig
		}
		copyConfig.NodesConfig[i] = &node
	}
	return &copyConfig
}

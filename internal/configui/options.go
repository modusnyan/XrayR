package configui

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/XrayR-project/XrayR/panel"
)

func panelOptions() []huh.Option[string] {
	definitions := panel.Panels()
	options := make([]huh.Option[string], 0, len(definitions))
	for _, definition := range definitions {
		options = append(options, huh.NewOption(definition.Name, definition.Name))
	}
	return options
}

func nodeTypeOptions(panelName string) []huh.Option[string] {
	definition, err := panel.LookupPanel(panelName)
	if err != nil {
		return huh.NewOptions("V2ray", "Vmess", "Vless", "Trojan", "Shadowsocks")
	}
	options := make([]huh.Option[string], 0, len(definition.NodeTypes))
	for _, nodeType := range definition.NodeTypes {
		options = append(options, huh.NewOption(nodeType, nodeType))
	}
	return options
}

func validateRequired(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}

func validateURL(value string) error {
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("remove leading or trailing whitespace")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("enter a valid HTTP or HTTPS URL")
	}
	return nil
}

func validatePositiveInt(value string) error {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fmt.Errorf("enter an integer greater than zero")
	}
	return nil
}

func validateNonNegativeInt(value string) error {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return fmt.Errorf("enter a non-negative integer")
	}
	return nil
}

func validateNonNegativeFloat(value string) error {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 {
		return fmt.Errorf("enter a non-negative number")
	}
	return nil
}

func validateListen(value string) error {
	if _, _, err := net.SplitHostPort(strings.TrimSpace(value)); err != nil {
		return fmt.Errorf("use host:port, for example 127.0.0.1:8080")
	}
	return nil
}

func validateEnv(value string) error {
	for _, line := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == ',' }) {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return fmt.Errorf("use one KEY=VALUE pair per line")
		}
	}
	return nil
}

func isVLESS(nodeType string, enableVless bool) bool {
	return strings.EqualFold(nodeType, "Vless") || (strings.EqualFold(nodeType, "V2ray") && enableVless)
}

func supportsFallback(nodeType string, enableVless bool) bool {
	return strings.EqualFold(nodeType, "Trojan") || isVLESS(nodeType, enableVless)
}

func acmeMode(mode string) bool {
	return mode == "dns" || mode == "http" || mode == "tls"
}

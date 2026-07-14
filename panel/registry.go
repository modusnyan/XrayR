package panel

import (
	"fmt"
	"sort"
	"strings"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/api/bunpanel"
	"github.com/XrayR-project/XrayR/api/gov2panel"
	"github.com/XrayR-project/XrayR/api/newV2board"
	"github.com/XrayR-project/XrayR/api/pmpanel"
	"github.com/XrayR-project/XrayR/api/proxypanel"
	"github.com/XrayR-project/XrayR/api/sspanel"
	"github.com/XrayR-project/XrayR/api/v2raysocks"
)

// ClientFactory creates a panel client from its API configuration.
type ClientFactory func(*api.Config) api.Client

// PanelDefinition describes a supported panel and its historical names.
type PanelDefinition struct {
	Name         string
	Adapter      string
	Aliases      []string
	NodeTypes    []string
	New          ClientFactory
	Capabilities api.PanelCapabilities
}

var panelDefinitions = []PanelDefinition{
	{
		Name: "SSpanel", Adapter: "SSpanel", New: func(c *api.Config) api.Client { return sspanel.New(c) },
		NodeTypes:    []string{"V2ray", "Vmess", "Vless", "Trojan", "Shadowsocks", "Shadowsocks-Plugin"},
		Capabilities: api.PanelCapabilities{Rules: true, TrafficReport: true, NodeStatusReport: true, OnlineUserReport: true, IllegalReport: true},
	},
	{
		Name: "Xboard", Adapter: "NewV2board", Aliases: []string{"NewV2board", "V2board"}, New: func(c *api.Config) api.Client { return newV2board.New(c) },
		NodeTypes:    []string{"V2ray", "Vmess", "Vless", "Trojan", "Shadowsocks"},
		Capabilities: api.PanelCapabilities{Rules: true, TrafficReport: true, Shadowsocks2022KeyDerivation: true},
	},
	{
		Name: "PMpanel", Adapter: "PMpanel", New: func(c *api.Config) api.Client { return pmpanel.New(c) },
		NodeTypes:    []string{"V2ray", "Vmess", "Vless", "Trojan", "Shadowsocks"},
		Capabilities: api.PanelCapabilities{Rules: true, TrafficReport: true, OnlineUserReport: true},
	},
	{
		Name: "Proxypanel", Adapter: "Proxypanel", New: func(c *api.Config) api.Client { return proxypanel.New(c) },
		NodeTypes:    []string{"V2ray", "Vmess", "Vless", "Trojan", "Shadowsocks"},
		Capabilities: api.PanelCapabilities{Rules: true, TrafficReport: true, NodeStatusReport: true, OnlineUserReport: true, IllegalReport: true},
	},
	{
		Name: "V2RaySocks", Adapter: "V2RaySocks", New: func(c *api.Config) api.Client { return v2raysocks.New(c) },
		NodeTypes:    []string{"V2ray", "Vmess", "Vless", "Trojan", "Shadowsocks"},
		Capabilities: api.PanelCapabilities{Rules: true, TrafficReport: true, NodeStatusReport: true, OnlineUserReport: true, IllegalReport: true},
	},
	{
		Name: "GoV2Panel", Adapter: "GoV2Panel", New: func(c *api.Config) api.Client { return gov2panel.New(c) },
		NodeTypes:    []string{"V2ray", "Vmess", "Vless", "Trojan", "Shadowsocks"},
		Capabilities: api.PanelCapabilities{Rules: true, TrafficReport: true},
	},
	{
		Name: "BunPanel", Adapter: "BunPanel", New: func(c *api.Config) api.Client { return bunpanel.New(c) },
		NodeTypes:    []string{"V2ray", "Vmess", "Vless", "Trojan", "Shadowsocks"},
		Capabilities: api.PanelCapabilities{Rules: true, TrafficReport: true, OnlineUserReport: true},
	},
}

// Panels returns a stable copy of the supported panel definitions.
func Panels() []PanelDefinition {
	definitions := append([]PanelDefinition(nil), panelDefinitions...)
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	return definitions
}

// LookupPanel resolves a canonical name or historical alias case-insensitively.
func LookupPanel(name string) (PanelDefinition, error) {
	needle := strings.TrimSpace(name)
	for _, definition := range panelDefinitions {
		if strings.EqualFold(needle, definition.Name) || strings.EqualFold(needle, definition.Adapter) {
			return definition, nil
		}
		for _, alias := range definition.Aliases {
			if strings.EqualFold(needle, alias) {
				return definition, nil
			}
		}
	}

	names := make([]string, 0, len(panelDefinitions))
	for _, definition := range Panels() {
		names = append(names, definition.Name)
	}
	if suggestion := suggestPanel(needle); suggestion != "" {
		return PanelDefinition{}, fmt.Errorf("unsupported panel type %q; did you mean %q? supported panels: %s", name, suggestion, strings.Join(names, ", "))
	}
	return PanelDefinition{}, fmt.Errorf("unsupported panel type %q; supported panels: %s", name, strings.Join(names, ", "))
}

// SupportsNodeType reports whether a panel supports a configured protocol.
func (d PanelDefinition) SupportsNodeType(nodeType string) bool {
	for _, supported := range d.NodeTypes {
		if strings.EqualFold(nodeType, supported) {
			return true
		}
	}
	return false
}

func suggestPanel(input string) string {
	input = strings.ToLower(input)
	bestName, bestDistance := "", 4
	for _, definition := range panelDefinitions {
		candidates := append([]string{definition.Name, definition.Adapter}, definition.Aliases...)
		for _, candidate := range candidates {
			if distance := levenshtein(input, strings.ToLower(candidate)); distance < bestDistance {
				bestName, bestDistance = definition.Name, distance
			}
		}
	}
	return bestName
}

func levenshtein(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = min3(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}

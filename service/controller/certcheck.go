package controller

import (
	"fmt"
	"strings"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/common/mylego"
)

// checkCertConfig warns when the node's TLS setup cannot work as the panel
// describes it. XrayR builds TLS from the local ControllerConfig.CertConfig only
// and never applies the panel's per-node cert_config, so a mismatch between the
// two is otherwise invisible. Each distinct set of warnings is logged once.
func (c *Controller) checkCertConfig() {
	var panel *api.PanelCertConfig
	if provider, ok := c.apiClient.(api.CertConfigProvider); ok {
		panel = provider.PanelCertConfig()
	}
	node := c.currentNode()
	localREALITY := node != nil && !node.EnableREALITY && !c.config.DisableLocalREALITYConfig &&
		c.config.EnableREALITY && c.config.REALITYConfigs != nil

	warnings := certConfigWarnings(node, c.config.CertConfig, localREALITY, panel)
	signature := strings.Join(warnings, "\n")
	if previous, _ := c.certWarnings.Load().(string); previous == signature {
		return
	}
	c.certWarnings.Store(signature)
	for _, warning := range warnings {
		c.logger.Warnf("Certificate configuration: %s", warning)
	}
}

// certConfigWarnings compares the local certificate config with the node and
// with the panel's cert_config. It only reports; nothing is changed.
func certConfigWarnings(node *api.NodeInfo, local *mylego.CertConfig, localREALITY bool, panel *api.PanelCertConfig) []string {
	// Only plain TLS nodes use a certificate; REALITY brings its own handshake.
	if node == nil || !node.EnableTLS || node.EnableREALITY || localREALITY {
		return nil
	}
	localMode := "none"
	if local != nil && strings.TrimSpace(local.CertMode) != "" {
		localMode = strings.ToLower(strings.TrimSpace(local.CertMode))
	}

	var warnings []string
	if localMode == "none" {
		warnings = append(warnings, "the node uses TLS in the panel but the local CertConfig is missing or has CertMode none, "+
			"so the inbound listens WITHOUT TLS and TLS clients cannot connect; configure ControllerConfig.CertConfig")
	}
	if panel == nil {
		return warnings
	}

	switch panel.Mode {
	case "self", "content":
		return append(warnings, fmt.Sprintf("the panel's cert mode %q is not supported by XrayR and is ignored; "+
			"XrayR uses the local CertConfig (CertMode %q)", panel.Mode, localMode))
	}
	if localMode == "none" {
		return append(warnings, fmt.Sprintf("the panel's cert_config (mode %q) is not applied by XrayR; "+
			"copy it into the local CertConfig", panel.Mode))
	}
	if panel.Mode != localMode {
		warnings = append(warnings, fmt.Sprintf("the panel's cert mode %q differs from the local CertMode %q; XrayR uses the local setting",
			panel.Mode, localMode))
	}
	if panel.Domain != "" && local.CertDomain != "" && !strings.EqualFold(panel.Domain, local.CertDomain) {
		warnings = append(warnings, fmt.Sprintf("the panel's cert domain %q differs from the local CertDomain %q; "+
			"clients connecting with the panel's domain will see a certificate for the local one", panel.Domain, local.CertDomain))
	}
	if panel.Mode == "dns" && localMode == "dns" && panel.Provider != "" && !strings.EqualFold(panel.Provider, local.Provider) {
		warnings = append(warnings, fmt.Sprintf("the panel's DNS provider %q differs from the local Provider %q; XrayR uses the local setting",
			panel.Provider, local.Provider))
	}
	return warnings
}

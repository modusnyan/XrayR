package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	xconfig "github.com/XrayR-project/XrayR/config"
)

func TestConfigInitNonInteractive(t *testing.T) {
	output := filepath.Join(t.TempDir(), "config.yml")
	command := newConfigInitCommand()
	command.SetArgs([]string{
		"--panel", "Xboard", "--api-host", "https://panel.example.com", "--api-key", "secret",
		"--node-id", "7", "--node-type", "Vless", "--output", output, "--skip-doctor",
	})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stdout)
	require.NoError(t, command.Execute())

	loaded, err := xconfig.Load(output)
	require.NoError(t, err)
	require.False(t, loaded.HasErrors())
	assert.Equal(t, "Xboard", loaded.Config.NodesConfig[0].PanelType)
	assert.Equal(t, "secret", loaded.Config.NodesConfig[0].ApiConfig.Key)
}

func TestConfigInitNonInteractiveDoesNotOverwrite(t *testing.T) {
	output := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(output, []byte("existing"), 0o600))
	command := newConfigInitCommand()
	command.SetArgs([]string{
		"--panel", "Xboard", "--api-host", "https://panel.example.com", "--api-key", "secret",
		"--node-id", "7", "--node-type", "Vless", "--output", output, "--skip-doctor",
	})
	assert.Error(t, command.Execute())
}

func TestConfigInitMissingRequiredFlagsWithoutTTY(t *testing.T) {
	command := newConfigInitCommand()
	command.SetIn(bytes.NewBuffer(nil))
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a terminal")
}

func TestConfigInitRequiredFlags(t *testing.T) {
	options := &configInitOptions{panelName: "Xboard", apiHost: "https://panel.example.com", apiKey: "key", nodeID: 1, nodeType: "Vless"}
	assert.True(t, options.hasRequiredFlags())
	options.apiKey = ""
	assert.False(t, options.hasRequiredFlags())
}

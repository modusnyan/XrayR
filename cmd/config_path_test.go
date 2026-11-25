package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirstExistingPrefersTheEarlierCandidate(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "etc.yml")
	local := filepath.Join(dir, "local.yml")
	for _, path := range []string{installed, local} {
		assert.NoError(t, os.WriteFile(path, []byte("ConfigVersion: 1\n"), 0o600))
	}

	assert.Equal(t, installed, firstExisting(installed, local))
}

func TestFirstExistingFallsBackToTheLastCandidate(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "missing.yml")
	local := filepath.Join(dir, "also-missing.yml")

	// Reporting the installed location is deliberate: an installed node should
	// hear about /etc/XrayR/config.yml, not about the working directory.
	assert.Equal(t, local, firstExisting(installed, local))
	assert.Equal(t, local, firstExisting(local))
}

func TestResolveConfigPathHonoursTheFlag(t *testing.T) {
	previous := cfgFile
	t.Cleanup(func() { cfgFile = previous })

	cfgFile = "/tmp/explicit.yml"
	assert.Equal(t, "/tmp/explicit.yml", resolveConfigPath())

	// Without -c the installed location wins when it exists, which is what makes
	// running the binary bare work on an installed node.
	cfgFile = ""
	dir := t.TempDir()
	installed := filepath.Join(dir, "installed.yml")
	assert.NoError(t, os.WriteFile(installed, []byte("ConfigVersion: 1\n"), 0o600))
	assert.Equal(t, installed, firstExisting(installed, localConfigPath))
}

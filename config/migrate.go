package config

import (
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/XrayR-project/XrayR/panel"
)

// Migrate normalizes a configuration to the current version and returns the
// migrated YAML plus compatibility warnings.
func Migrate(reader io.Reader) ([]byte, []Issue, error) {
	result, err := Decode(reader)
	if err != nil {
		return nil, nil, err
	}
	ApplyDefaults(result.Config, ".")
	result.Config.ConfigVersion = CurrentVersion
	for _, node := range result.Config.NodesConfig {
		if node == nil {
			continue
		}
		if definition, lookupErr := panel.LookupPanel(node.PanelType); lookupErr == nil {
			node.PanelType = definition.Name
		}
	}
	data, err := yaml.Marshal(result.Config)
	return data, result.Issues, err
}

// WriteAtomic writes a private configuration file without exposing a partial file.
func WriteAtomic(path string, data []byte, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return os.ErrExist
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".xrayr-config-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

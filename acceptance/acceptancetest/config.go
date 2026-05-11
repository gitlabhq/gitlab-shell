//go:build acceptance

package acceptancetest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// configFields is the harness-internal subset of internal/config.Config
// that the acceptance suite cares about. Marshalled to YAML and written
// as <dir>/config.yml.
//
// New fields are added as new acceptance tests need them.
type configFields struct {
	GitlabURL string `yaml:"gitlab_url"`
	Secret    string `yaml:"secret"`
}

// writeConfigDir writes a config.yml under dir. The dir must already
// exist (typically t.TempDir()).
func writeConfigDir(dir string, fields configFields) error {
	out, err := yaml.Marshal(fields)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("write config to %s: %w", path, err)
	}
	return nil
}

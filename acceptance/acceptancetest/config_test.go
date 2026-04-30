//go:build acceptance

package acceptancetest

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func TestWriteConfigDir_writesParseableYAML(t *testing.T) {
	dir := t.TempDir()

	err := writeConfigDir(dir, configFields{
		GitlabURL: "http://localhost:1234",
		Secret:    "test-secret",
	})
	require.NoError(t, err)

	cfg, err := config.NewFromDir(dir)
	require.NoError(t, err)

	require.Equal(t, "http://localhost:1234", cfg.GitlabURL)
	require.Equal(t, "test-secret", cfg.Secret)
}

func TestWriteConfigDir_filenameIsConfigYAML(t *testing.T) {
	dir := t.TempDir()
	err := writeConfigDir(dir, configFields{GitlabURL: "http://x", Secret: "s"})
	require.NoError(t, err)

	matches, err := filepath.Glob(filepath.Join(dir, "config.yml"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
}

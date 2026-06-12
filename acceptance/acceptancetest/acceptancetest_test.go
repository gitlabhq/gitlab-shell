//go:build acceptance

package acceptancetest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun_smokeTestGitlabShellCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v4/internal/check", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"api_version":"v4","redis":true}`))
	}))
	t.Cleanup(srv.Close)

	res := Run(t, Config{
		Binary:         "gitlab-shell-check",
		InternalAPIURL: srv.URL,
		Secret:         "test-secret",
	})

	require.Equal(t, 0, res.ExitCode, "stderr: %s", res.Stderr)
	require.Contains(t, res.Stdout, "Internal API available: OK")
	require.Contains(t, res.Stdout, "Redis available via internal API: OK")
}

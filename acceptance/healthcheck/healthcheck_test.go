//go:build acceptance

package healthcheck_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/acceptance/acceptancetest"
)

// TestHealthcheck runs the gitlab-shell-check binary against a fake internal API
// and asserts exit code and output. The health check always uses the new gitlab
// client, so there is no feature-flag dimension.
func TestHealthcheck(t *testing.T) {
	apiCases := []struct {
		name         string
		apiStatus    int
		apiBody      string
		wantExitZero bool
		gateAuth     bool
	}{
		{"api_healthy", http.StatusOK, `{"api_version":"v4","redis":true}`, true, true},
		{"api_500", http.StatusInternalServerError, `boom`, false, false},
	}

	for _, api := range apiCases {
		t.Run(api.name, func(t *testing.T) {
			var captured *http.Request
			var apiURL string
			if api.gateAuth {
				apiURL = authGatedHealthcheckEndpoint(t, "test-secret", api.apiBody, &captured)
			} else {
				mux := http.NewServeMux()
				mux.HandleFunc("/api/v4/internal/check", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(api.apiStatus)
					fmt.Fprint(w, api.apiBody)
				})
				apiURL = newFakeServer(t, mux)
			}

			cfg := acceptancetest.Config{
				Binary:         "gitlab-shell-check",
				InternalAPIURL: apiURL,
				Secret:         "test-secret",
			}

			res := acceptancetest.Run(t, cfg)

			if api.wantExitZero {
				require.Equal(t, 0, res.ExitCode, "stderr: %s\nstdout: %s", res.Stderr, res.Stdout)
				require.Contains(t, res.Stdout, "Internal API available: OK")
			} else {
				require.NotEqual(t, 0, res.ExitCode, "expected non-zero; stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
				require.Contains(t, res.Stderr, "Internal API available: FAILED")
			}

			if api.gateAuth {
				require.NotNil(t, captured, "expected request to be captured")
				require.Equal(t, "GitLab-Shell", captured.Header.Get("User-Agent"))
			}
		})
	}
}

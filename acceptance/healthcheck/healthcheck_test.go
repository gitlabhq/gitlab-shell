//go:build acceptance

package healthcheck_test

import (
	"net/http"
	"testing"

	"github.com/elliotforbes/fakes"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/acceptance/acceptancetest"
)

func TestHealthcheck_FFOff(t *testing.T) {
	cases := []struct {
		name         string
		apiStatus    int
		apiBody      string
		wantExitZero bool
	}{
		{"api_healthy", http.StatusOK, `{"api_version":"v4","redis":true}`, true},
		{"api_500", http.StatusInternalServerError, `boom`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := fakes.New().Endpoint(&fakes.Endpoint{
				Path:       "/api/v4/internal/check",
				Methods:    []string{http.MethodGet, http.MethodPost},
				StatusCode: tc.apiStatus,
				Response:   tc.apiBody,
			})
			api.Run(t)

			res := acceptancetest.Run(t, acceptancetest.Config{
				Binary:         "gitlab-shell-check",
				InternalAPIURL: api.BaseURL,
				Secret:         "test-secret",
				// FeatureFlagURL deliberately unset: exercises the
				// "old client" fallback when no FF service is configured.
			})

			if tc.wantExitZero {
				require.Equal(t, 0, res.ExitCode, "stderr: %s\nstdout: %s", res.Stderr, res.Stdout)
				require.Contains(t, res.Stdout, "Internal API available: OK")
			} else {
				require.NotEqual(t, 0, res.ExitCode, "expected non-zero; stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
				require.Contains(t, res.Stderr, "Internal API available: FAILED")
			}
		})
	}
}

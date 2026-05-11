//go:build acceptance

package healthcheck_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/acceptance/acceptancetest"
)

// fakeFliptEndpoint starts a fake Flipt boolean evaluation server.
// enabled drives the response: true routes the binary to the new client,
// false routes to the old client.
func fakeFliptEndpoint(t *testing.T, enabled bool) string {
	mux := http.NewServeMux()
	mux.HandleFunc("/evaluate/v1/boolean", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"enabled":%t,"flag_key":"use_new_healthcheck_client"}`, enabled)
	})
	return newFakeServer(t, mux)
}

func TestHealthcheck_FFOff(t *testing.T) {
	cases := []struct {
		name         string
		apiStatus    int
		apiBody      string
		wantExitZero bool
		gateAuth     bool
	}{
		{"api_healthy", http.StatusOK, `{"api_version":"v4","redis":true}`, true, true},
		{"api_500", http.StatusInternalServerError, `boom`, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured *http.Request
			var apiURL string
			if tc.gateAuth {
				apiURL = authGatedHealthcheckEndpoint(t, "test-secret", tc.apiBody, &captured)
			} else {
				mux := http.NewServeMux()
				mux.HandleFunc("/api/v4/internal/check", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.apiStatus)
					fmt.Fprint(w, tc.apiBody)
				})
				apiURL = newFakeServer(t, mux)
			}

			res := acceptancetest.Run(t, acceptancetest.Config{
				Binary:         "gitlab-shell-check",
				InternalAPIURL: apiURL,
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

			if tc.gateAuth {
				require.NotNil(t, captured, "expected request to be captured")
				require.Equal(t, "GitLab-Shell", captured.Header.Get("User-Agent"))
			}
		})
	}
}

func TestHealthcheck_FFOn(t *testing.T) {
	cases := []struct {
		name         string
		apiStatus    int
		apiBody      string
		wantExitZero bool
		gateAuth     bool
	}{
		{"api_healthy", http.StatusOK, `{"api_version":"v4","redis":true}`, true, true},
		{"api_500", http.StatusInternalServerError, `boom`, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ffURL := fakeFliptEndpoint(t, true)

			var captured *http.Request
			var apiURL string
			if tc.gateAuth {
				apiURL = authGatedHealthcheckEndpoint(t, "test-secret", tc.apiBody, &captured)
			} else {
				mux := http.NewServeMux()
				mux.HandleFunc("/api/v4/internal/check", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.apiStatus)
					fmt.Fprint(w, tc.apiBody)
				})
				apiURL = newFakeServer(t, mux)
			}

			res := acceptancetest.Run(t, acceptancetest.Config{
				Binary:         "gitlab-shell-check",
				InternalAPIURL: apiURL,
				FeatureFlagURL: ffURL,
				Secret:         "test-secret",
			})

			if tc.wantExitZero {
				require.Equal(t, 0, res.ExitCode, "stderr: %s\nstdout: %s", res.Stderr, res.Stdout)
				require.Contains(t, res.Stdout, "Internal API available: OK")
			} else {
				require.NotEqual(t, 0, res.ExitCode, "expected non-zero; stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			}

			if tc.gateAuth {
				require.NotNil(t, captured, "expected request to be captured")
				require.Equal(t, "GitLab-Shell", captured.Header.Get("User-Agent"))
			}
		})
	}
}

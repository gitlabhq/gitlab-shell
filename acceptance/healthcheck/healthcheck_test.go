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
// false routes to the old client (the FF-configured-but-disabled branch).
func fakeFliptEndpoint(t *testing.T, enabled bool) string {
	mux := http.NewServeMux()
	mux.HandleFunc("/evaluate/v1/boolean", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"enabled":%t,"flag_key":"use_new_healthcheck_client"}`, enabled)
	})
	return newFakeServer(t, mux)
}

// ffState captures how the feature-flag service is wired for a test case.
// The three states map to the three branches of runCheck's dispatch:
//
//	ffUnconfigured       — no FF service in context, falls through to legacy
//	ffServiceReturnsFalse — FF service answers enabled=false, falls through to legacy
//	ffServiceReturnsTrue  — FF service answers enabled=true, uses the new client
type ffState int

const (
	ffUnconfigured ffState = iota
	ffServiceReturnsFalse
	ffServiceReturnsTrue
)

func (s ffState) String() string {
	switch s {
	case ffUnconfigured:
		return "ff_unconfigured"
	case ffServiceReturnsFalse:
		return "ff_service_returns_false"
	case ffServiceReturnsTrue:
		return "ff_service_returns_true"
	}
	return "unknown"
}

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

	ffCases := []ffState{ffUnconfigured, ffServiceReturnsFalse, ffServiceReturnsTrue}

	for _, ff := range ffCases {
		for _, api := range apiCases {
			t.Run(ff.String()+"/"+api.name, func(t *testing.T) {
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
				switch ff {
				case ffServiceReturnsFalse:
					cfg.FeatureFlagURL = fakeFliptEndpoint(t, false)
				case ffServiceReturnsTrue:
					cfg.FeatureFlagURL = fakeFliptEndpoint(t, true)
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
}

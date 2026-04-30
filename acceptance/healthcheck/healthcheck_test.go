//go:build acceptance

package healthcheck_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/elliotforbes/fakes"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/acceptance/acceptancetest"
)

// fakeFliptEndpoint returns a fakes.Endpoint that responds to the
// POST /evaluate/v1/boolean call that labkit/v2/featureflag's Flipt
// provider makes. enabled drives the response: true routes the binary
// to the new client, false routes to the old client.
//
// The payload is the smallest valid Flipt boolean evaluation response —
// protojson with DiscardUnknown will accept any superset.
func fakeFliptEndpoint(enabled bool) *fakes.Endpoint {
	return &fakes.Endpoint{
		Path:       "/evaluate/v1/boolean",
		Methods:    []string{http.MethodPost},
		StatusCode: http.StatusOK,
		Response:   fmt.Sprintf(`{"enabled":%t,"flag_key":"use_new_healthcheck_client"}`, enabled),
	}
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
			var endpoint *fakes.Endpoint
			if tc.gateAuth {
				endpoint = authGatedHealthcheckEndpoint("test-secret", tc.apiBody, &captured)
			} else {
				endpoint = &fakes.Endpoint{
					Path:       "/api/v4/internal/check",
					Methods:    []string{http.MethodGet, http.MethodPost},
					StatusCode: tc.apiStatus,
					Response:   tc.apiBody,
				}
			}

			api := fakes.New().Endpoint(endpoint)
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
			ff := fakes.New().Endpoint(fakeFliptEndpoint(true))
			ff.Run(t)

			var captured *http.Request
			var endpoint *fakes.Endpoint
			if tc.gateAuth {
				endpoint = authGatedHealthcheckEndpoint("test-secret", tc.apiBody, &captured)
			} else {
				endpoint = &fakes.Endpoint{
					Path:       "/api/v4/internal/check",
					Methods:    []string{http.MethodGet, http.MethodPost},
					StatusCode: tc.apiStatus,
					Response:   tc.apiBody,
				}
			}

			api := fakes.New().Endpoint(endpoint)
			api.Run(t)

			res := acceptancetest.Run(t, acceptancetest.Config{
				Binary:         "gitlab-shell-check",
				InternalAPIURL: api.BaseURL,
				FeatureFlagURL: ff.BaseURL,
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

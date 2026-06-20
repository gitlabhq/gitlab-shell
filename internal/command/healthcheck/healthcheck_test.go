package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/healthcheck"
	"gitlab.com/gitlab-org/labkit/v2/httpclient"
)

const (
	testHealthyOutput = "Internal API available: OK\nRedis available via internal API: OK\n"
	testSecret        = "test-secret"
)

// mockEvaluator is a test double for featureflag.Evaluator.
type mockEvaluator struct {
	value bool
	err   error
}

func (m *mockEvaluator) BooleanValueDetails(_ context.Context, _ string, _ bool, _ openfeature.EvaluationContext, _ ...openfeature.Option) (openfeature.BooleanEvaluationDetails, error) {
	return openfeature.BooleanEvaluationDetails{Value: m.value}, m.err
}

func (m *mockEvaluator) StringValueDetails(_ context.Context, _ string, defaultValue string, _ openfeature.EvaluationContext, _ ...openfeature.Option) (openfeature.StringEvaluationDetails, error) {
	return openfeature.StringEvaluationDetails{Value: defaultValue}, nil
}

// fastRetryOpts returns HTTPClientOpts and NewClientRetryConfig that use
// near-zero backoff delays so that tests exercising retry paths (e.g. 500
// responses) complete in milliseconds instead of seconds.
func fastRetryOpts() ([]client.HTTPClientOpt, *httpclient.RetryConfig) {
	legacyOpts := []client.HTTPClientOpt{
		client.WithHTTPRetryOpts(time.Millisecond, time.Millisecond, 2),
	}
	newClientCfg := &httpclient.RetryConfig{
		MaxAttempts:     3,
		RetryableStatus: []int{http.StatusInternalServerError},
		RetryOnError:    true,
		BaseDelay:       time.Millisecond,
		MaxDelay:        time.Millisecond,
	}
	return legacyOpts, newClientCfg
}

// checkHandlers builds test handlers for /api/v4/internal/check.
// Both the legacy client and the new gitlab client normalise their paths to
// /api/v4/internal/check, so a single set of handlers covers both code paths.
func checkHandlers(code int, rsp *healthcheck.Response) []testserver.TestRequestHandler {
	return []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/check",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				if rsp != nil {
					json.NewEncoder(w).Encode(rsp)
				}
			},
		},
	}
}

var (
	okResponse = &healthcheck.Response{
		APIVersion:     "v4",
		GitlabVersion:  "v12.0.0-ee",
		GitlabRevision: "3b13818e8330f68625d80d9bf5d8049c41fbe197",
		Redis:          true,
	}
	badRedisResponse = &healthcheck.Response{Redis: false}
)

// TestClientDispatch verifies that runCheck routes to the correct underlying
// client based on the feature flag evaluator in context.
func TestClientDispatch(t *testing.T) {
	tests := []struct {
		name      string
		evaluator *mockEvaluator
		wantOut   string
		wantErr   string
	}{
		{
			name:      "no evaluator in context — uses legacy client",
			evaluator: nil,
			wantOut:   testHealthyOutput,
		},
		{
			name:      "evaluator present but flag off — uses legacy client",
			evaluator: &mockEvaluator{value: false},
			wantOut:   testHealthyOutput,
		},
		{
			name:      "evaluator present and flag on — uses new client",
			evaluator: &mockEvaluator{value: true},
			wantOut:   testHealthyOutput,
		},
		{
			name:      "evaluator errors — warns and falls back to legacy client",
			evaluator: &mockEvaluator{err: errors.New("ff service unavailable")},
			wantOut:   testHealthyOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := testserver.StartSocketHTTPServer(t, checkHandlers(200, okResponse))

			buffer := &bytes.Buffer{}
			cmd := &Command{
				// Secret is required by the new client; harmless for the legacy client.
				Config:     &config.Config{GitlabURL: url, Secret: testSecret},
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			ctx := context.Background()
			if tt.evaluator != nil {
				ctx = command.ContextWithEvaluator(ctx, tt.evaluator)
			}

			_, err := cmd.Execute(ctx)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantOut, buffer.String())
		})
	}
}

// TestLegacyClientResponses verifies Execute output for every response shape the
// legacy client can receive, including construction failure.
func TestLegacyClientResponses(t *testing.T) {
	tests := []struct {
		name       string
		gitlabURL  string // leave empty to use a live test server
		handlers   []testserver.TestRequestHandler
		wantOut    string
		wantErrMsg string
	}{
		{
			name:     "api and redis both healthy",
			handlers: checkHandlers(200, okResponse),
			wantOut:  testHealthyOutput,
		},
		{
			name:       "api healthy but redis unavailable",
			handlers:   checkHandlers(200, badRedisResponse),
			wantOut:    "Internal API available: OK\n",
			wantErrMsg: "Redis available via internal API: FAILED",
		},
		{
			name:       "api returns 500",
			handlers:   checkHandlers(500, nil),
			wantErrMsg: "Internal API available: FAILED - Internal API unreachable",
		},
		{
			name:       "unsupported protocol — client construction fails",
			gitlabURL:  "ftp://unsupported.invalid",
			wantErrMsg: "Internal API available: FAILED - error creating http client: unknown GitLab URL prefix",
		},
	}

	legacyOpts, _ := fastRetryOpts()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.gitlabURL
			if url == "" {
				url = testserver.StartSocketHTTPServer(t, tt.handlers)
			}

			buffer := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabURL: url, HTTPClientOpts: legacyOpts},
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			_, err := cmd.Execute(context.Background())

			if tt.wantErrMsg != "" {
				require.EqualError(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantOut, buffer.String())
		})
	}
}

// TestNewClientResponses verifies Execute output for every response shape the
// new gitlab client can receive when the feature flag is enabled.
func TestNewClientResponses(t *testing.T) {
	tests := []struct {
		name       string
		secret     string
		handlers   []testserver.TestRequestHandler
		wantOut    string
		wantErrMsg string
	}{
		{
			name:     "api and redis both healthy",
			secret:   testSecret,
			handlers: checkHandlers(200, okResponse),
			wantOut:  testHealthyOutput,
		},
		{
			name:       "api healthy but redis unavailable",
			secret:     testSecret,
			handlers:   checkHandlers(200, badRedisResponse),
			wantOut:    "Internal API available: OK\n",
			wantErrMsg: "Redis available via internal API: FAILED",
		},
		{
			name:       "api returns 500",
			secret:     testSecret,
			handlers:   checkHandlers(500, nil),
			wantErrMsg: "Internal API available: FAILED - Internal API error (500)",
		},
		{
			name:       "client construction fails — secret is empty",
			secret:     "",
			wantErrMsg: "Internal API available: FAILED - secret must not be empty",
		},
	}

	_, newClientCfg := fastRetryOpts()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var url string
			if tt.handlers != nil {
				url = testserver.StartSocketHTTPServer(t, tt.handlers)
			} else {
				url = "http+unix:///dev/null"
			}

			buffer := &bytes.Buffer{}
			cmd := &Command{
				Config:     &config.Config{GitlabURL: url, Secret: tt.secret, NewClientRetryConfig: newClientCfg},
				ReadWriter: &readwriter.ReadWriter{Out: buffer},
			}

			ctx := command.ContextWithEvaluator(context.Background(), &mockEvaluator{value: true})
			_, err := cmd.Execute(ctx)

			if tt.wantErrMsg != "" {
				require.EqualError(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantOut, buffer.String())
		})
	}
}

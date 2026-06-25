package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// checkHandlers builds test handlers for /api/v4/internal/check. The new gitlab
// client normalises its path to /api/v4/internal/check.
func checkHandlers(code int, rsp *gitlab.HealthcheckResponse) []testserver.TestRequestHandler {
	return []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/check",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				if rsp != nil {
					_ = json.NewEncoder(w).Encode(rsp)
				}
			},
		},
	}
}

var (
	okResponse = &gitlab.HealthcheckResponse{
		APIVersion:     "v4",
		GitlabVersion:  "v12.0.0-ee",
		GitlabRevision: "3b13818e8330f68625d80d9bf5d8049c41fbe197",
		Redis:          true,
	}
	badRedisResponse = &gitlab.HealthcheckResponse{Redis: false}
)

// TestHealthcheckResponses verifies Execute output for every response shape the
// gitlab client can receive, including construction failure.
func TestHealthcheckResponses(t *testing.T) {
	tests := []struct {
		name       string
		secret     string
		handlers   []testserver.TestRequestHandler
		wantOut    string
		wantErrMsg string
	}{
		{
			name:     "api and redis both healthy",
			secret:   "test-secret",
			handlers: checkHandlers(200, okResponse),
			wantOut:  "Internal API available: OK\nRedis available via internal API: OK\n",
		},
		{
			name:       "api healthy but redis unavailable",
			secret:     "test-secret",
			handlers:   checkHandlers(200, badRedisResponse),
			wantOut:    "Internal API available: OK\n",
			wantErrMsg: "Redis available via internal API: FAILED",
		},
		{
			name:       "api returns 500",
			secret:     "test-secret",
			handlers:   checkHandlers(500, nil),
			wantErrMsg: "Internal API available: FAILED - Internal API error (500)",
		},
		{
			name:       "client construction fails — secret is empty",
			secret:     "",
			wantErrMsg: "Internal API available: FAILED - secret must not be empty",
		},
	}

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
				Config:     &config.Config{GitlabURL: url, Secret: tt.secret},
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

package healthcheck

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/healthcheck"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
)

var (
	okResponse = &healthcheck.Response{
		APIVersion:     "v4",
		GitlabVersion:  "v12.0.0-ee",
		GitlabRevision: "3b13818e8330f68625d80d9bf5d8049c41fbe197",
		Redis:          true,
	}

	badRedisResponse = &healthcheck.Response{Redis: false}

	okHandlers       = buildTestHandlers(200, okResponse)
	badRedisHandlers = buildTestHandlers(200, badRedisResponse)
	brokenHandlers   = buildTestHandlers(500, nil)
)

func buildTestHandlers(code int, rsp *healthcheck.Response) []testserver.TestRequestHandler {
	return []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/check",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				if rsp != nil {
					json.NewEncoder(w).Encode(rsp)
				}
			},
		},
	}
}

func TestExecute(t *testing.T) {
	url, cleanup := testserver.StartSocketHttpServer(t, okHandlers)
	defer cleanup()

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	err := cmd.Execute()

	require.NoError(t, err)
	require.Equal(t, "Internal API available: OK\nRedis available via internal API: OK\n", buffer.String())
}

func TestFailingRedisExecute(t *testing.T) {
	url, cleanup := testserver.StartSocketHttpServer(t, badRedisHandlers)
	defer cleanup()

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	err := cmd.Execute()
	require.Error(t, err, "Redis available via internal API: FAILED")
	require.Equal(t, "Internal API available: OK\n", buffer.String())
}

func TestFailingAPIExecute(t *testing.T) {
	url, cleanup := testserver.StartSocketHttpServer(t, brokenHandlers)
	defer cleanup()

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	err := cmd.Execute()
	require.Empty(t, buffer.String())
	require.EqualError(t, err, "Internal API available: FAILED - Internal API error (500)")
}

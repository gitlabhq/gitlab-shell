package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/healthcheck"
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
	url := testserver.StartSocketHttpServer(t, okHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	err := cmd.Execute(context.Background())

	require.NoError(t, err)
	require.Equal(t, "Internal API available: OK\nRedis available via internal API: OK\n", buffer.String())
}

func TestFailingRedisExecute(t *testing.T) {
	url := testserver.StartSocketHttpServer(t, badRedisHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	err := cmd.Execute(context.Background())
	require.Error(t, err, "Redis available via internal API: FAILED")
	require.Equal(t, "Internal API available: OK\n", buffer.String())
}

func TestFailingAPIExecute(t *testing.T) {
	url := testserver.StartSocketHttpServer(t, brokenHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	err := cmd.Execute(context.Background())
	require.Empty(t, buffer.String())
	require.EqualError(t, err, "Internal API available: FAILED - Internal API unreachable")
}

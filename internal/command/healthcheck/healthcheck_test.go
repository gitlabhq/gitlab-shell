package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/healthcheck"
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
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				if rsp != nil {
					json.NewEncoder(w).Encode(rsp)
				}
			},
		},
	}
}

func TestExecute(t *testing.T) {
	url := testserver.StartSocketHTTPServer(t, okHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	_, err := cmd.Execute(context.Background())

	require.NoError(t, err)
	require.Equal(t, "Internal API available: OK\nRedis available via internal API: OK\n", buffer.String())
}

func TestFailingRedisExecute(t *testing.T) {
	url := testserver.StartSocketHTTPServer(t, badRedisHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	_, err := cmd.Execute(context.Background())
	require.Error(t, err, "Redis available via internal API: FAILED")
	require.Equal(t, "Internal API available: OK\n", buffer.String())
}

func TestFailingAPIExecute(t *testing.T) {
	url := testserver.StartSocketHTTPServer(t, brokenHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	_, err := cmd.Execute(context.Background())
	require.Empty(t, buffer.String())
	require.EqualError(t, err, "Internal API available: FAILED - Internal API unreachable")
}

func TestExecuteWithFeatureFlagEnabled(t *testing.T) {
	// Both clients normalise the path to /api/v4/internal/check, so okHandlers works for both.
	url := testserver.StartSocketHTTPServer(t, okHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabURL: url, Secret: "test-secret"},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	ctx := command.ContextWithEvaluator(context.Background(), &mockEvaluator{value: true})
	_, err := cmd.Execute(ctx)

	require.NoError(t, err)
	require.Equal(t, "Internal API available: OK\nRedis available via internal API: OK\n", buffer.String())
}

func TestExecuteWithFeatureFlagDisabled(t *testing.T) {
	// Flag is false — must use old client path (/api/v4/internal/check).
	url := testserver.StartSocketHTTPServer(t, okHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	ctx := command.ContextWithEvaluator(context.Background(), &mockEvaluator{value: false})
	_, err := cmd.Execute(ctx)

	require.NoError(t, err)
	require.Equal(t, "Internal API available: OK\nRedis available via internal API: OK\n", buffer.String())
}

func TestExecuteWithFeatureFlagEvaluationError(t *testing.T) {
	// Evaluation error — must fall back to old client and not propagate the error.
	url := testserver.StartSocketHTTPServer(t, okHandlers)

	buffer := &bytes.Buffer{}
	cmd := &Command{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: buffer},
	}

	ctx := command.ContextWithEvaluator(context.Background(), &mockEvaluator{err: errors.New("ff service unavailable")})
	_, err := cmd.Execute(ctx)

	require.NoError(t, err)
	require.Equal(t, "Internal API available: OK\nRedis available via internal API: OK\n", buffer.String())
}

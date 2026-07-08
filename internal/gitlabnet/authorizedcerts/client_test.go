package authorizedcerts

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
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

func certHandlers(body string) []testserver.TestRequestHandler {
	return []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_certs",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			},
		},
	}
}

// TestGetByKeyDispatch verifies both the old and new client paths produce the
// same parsed Response, regardless of which the feature flag selects.
func TestGetByKeyDispatch(t *testing.T) {
	tests := []struct {
		name      string
		evaluator *mockEvaluator
	}{
		{"no evaluator — legacy client", nil},
		{"flag off — legacy client", &mockEvaluator{value: false}},
		{"flag on — new client", &mockEvaluator{value: true}},
		{"evaluator errors — legacy fallback", &mockEvaluator{err: errors.New("ff down")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := testserver.StartSocketHTTPServer(t, certHandlers(`{"username":"alice","namespace":"alice-group"}`))

			cfg := &config.Config{GitlabURL: url, Secret: "test-secret"}
			client, err := NewClient(cfg)
			require.NoError(t, err)

			ctx := context.Background()
			if tt.evaluator != nil {
				ctx = command.ContextWithEvaluator(ctx, tt.evaluator)
			}

			resp, err := client.GetByKey(ctx, "user-1", "fp-abc")
			require.NoError(t, err)
			require.Equal(t, "alice", resp.Username)
			require.Equal(t, "alice-group", resp.Namespace)
		})
	}
}

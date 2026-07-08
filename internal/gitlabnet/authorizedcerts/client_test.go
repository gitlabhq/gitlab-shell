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

// TestNewClientNoSecret verifies that NewClient succeeds even when no secret is
// configured (as in many unit-test server configs). It also verifies that
// GetByKey correctly falls back to the legacy path when the unified client is
// nil (flag on but newClient == nil must not panic or error).
func TestNewClientNoSecret(t *testing.T) {
	url := testserver.StartSocketHTTPServer(t, certHandlers(`{"username":"bob","namespace":"bob-group"}`))

	// No Secret field — this previously caused "secret must not be empty".
	cfg := &config.Config{GitlabURL: url}
	client, err := NewClient(cfg)
	require.NoError(t, err, "NewClient must succeed without a secret")
	require.NotNil(t, client)

	// Even with the flag evaluator returning true, newClient is nil so the
	// legacy path must be used and the response must be parsed correctly.
	ctx := command.ContextWithEvaluator(context.Background(), &mockEvaluator{value: true})
	resp, err := client.GetByKey(ctx, "user-2", "fp-xyz")
	require.NoError(t, err)
	require.Equal(t, "bob", resp.Username)
	require.Equal(t, "bob-group", resp.Namespace)
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

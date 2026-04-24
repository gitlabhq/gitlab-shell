package command

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

func TestSetup(t *testing.T) {
	testCases := []struct {
		name                  string
		additionalEnv         map[string]string
		expectedCorrelationID string
	}{
		{
			name: "no CORRELATION_ID in environment",
		},
		{
			name: "CORRELATION_ID in environment",
			additionalEnv: map[string]string{
				"CORRELATION_ID": "abc123",
			},
			expectedCorrelationID: "abc123",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addAdditionalEnv(t, tc.additionalEnv)

			ctx, finished := Setup("foo", &config.Config{})
			defer finished()

			require.NotNil(t, ctx, "ctx is nil")
			require.NotNil(t, finished, "finished is nil")

			correlationID := correlation.ExtractFromContext(ctx)
			require.NotEmpty(t, correlationID)
			if tc.expectedCorrelationID != "" {
				require.Equal(t, tc.expectedCorrelationID, correlationID)
			}

			clientName := correlation.ExtractClientNameFromContext(ctx)
			require.Equal(t, "foo", clientName)
		})
	}
}

func TestSetupConfiguresFeatureFlagClientWithTimeout(t *testing.T) {
	t.Setenv(featureFlagEndpointEnv, "http://localhost:8080")

	ctx, finished := Setup("test-service", &config.Config{})
	defer finished()

	// Verify that the feature flag evaluator is present in context
	evaluator := FeatureFlagEvaluatorFromContext(ctx)
	require.NotNil(t, evaluator, "feature flag evaluator should be present in context")
}

func TestSetupWithoutFeatureFlagEndpoint(t *testing.T) {
	// Ensure the environment variable is not set
	t.Setenv(featureFlagEndpointEnv, "")

	ctx, finished := Setup("test-service", &config.Config{})
	defer finished()

	// Verify that the context is still valid
	require.NotNil(t, ctx, "context should be created even without feature flag endpoint")

	// Verify that the feature flag evaluator is nil (graceful degradation)
	evaluator := FeatureFlagEvaluatorFromContext(ctx)
	require.Nil(t, evaluator, "feature flag evaluator should be nil when endpoint is not configured")

	// Verify correlation ID is still set
	correlationID := correlation.ExtractFromContext(ctx)
	require.NotEmpty(t, correlationID, "correlation ID should be set")
}

func TestSetupWithUnreachableFeatureFlagService(t *testing.T) {
	// Use an unreachable endpoint that will trigger the 1-second timeout
	t.Setenv(featureFlagEndpointEnv, "http://192.0.2.1:9999") // TEST-NET-1 reserved range, guaranteed unreachable

	ctx, finished := Setup("test-service", &config.Config{})
	defer finished()

	// Verify that the context is still valid
	require.NotNil(t, ctx, "context should be created even if service is unreachable")

	// Verify that the feature flag evaluator is present (client initializes without validating connectivity)
	// The client will fail gracefully on evaluation attempts if the service is unavailable
	evaluator := FeatureFlagEvaluatorFromContext(ctx)
	require.NotNil(t, evaluator, "feature flag evaluator should be created even when service is unreachable")

	// Verify correlation ID is still set
	correlationID := correlation.ExtractFromContext(ctx)
	require.NotEmpty(t, correlationID, "correlation ID should be set")
}

func TestSetupAttachesCorrelationIDToLogger(t *testing.T) {
	testCases := []struct {
		name          string
		additionalEnv map[string]string
	}{
		{name: "generates a new correlation ID when none is set"},
		{
			name:          "uses the correlation ID from the environment",
			additionalEnv: map[string]string{"CORRELATION_ID": "abc123"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addAdditionalEnv(t, tc.additionalEnv)

			var buf bytes.Buffer
			originalDefault := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
			t.Cleanup(func() { slog.SetDefault(originalDefault) })

			ctx, finished := Setup("foo", &config.Config{})
			defer finished()

			log.FromContext(ctx).InfoContext(ctx, "marker")

			var entry map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

			require.Equal(t, correlation.ExtractFromContext(ctx), entry["correlation_id"])
			require.NotEmpty(t, entry["correlation_id"])
		})
	}
}

// addAdditionalEnv will configure additional environment values
// and return a deferrable function to reset the environment to
// it's original state after the test
func addAdditionalEnv(t *testing.T, envMap map[string]string) {
	for k, v := range envMap {
		t.Setenv(k, v)
	}
}

func TestNewLogData(t *testing.T) {
	testCases := []struct {
		desc                  string
		project               string
		username              string
		expectedRootNamespace string
		projectID             int
		rootNamespaceID       int
	}{
		{
			desc:                  "Project under single namespace",
			project:               "flightjs/Flight",
			username:              "alex-doe",
			expectedRootNamespace: "flightjs",
			projectID:             1,
			rootNamespaceID:       2,
		},
		{
			desc:                  "Project under single odd namespace",
			project:               "flightjs///Flight",
			username:              "alex-doe",
			expectedRootNamespace: "flightjs",
			projectID:             1,
			rootNamespaceID:       2,
		},
		{
			desc:                  "Project under deeper namespace",
			project:               "flightjs/one/Flight",
			username:              "alex-doe",
			expectedRootNamespace: "flightjs",
			projectID:             1,
			rootNamespaceID:       2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			data := NewLogData(tc.project, tc.username, tc.projectID, tc.rootNamespaceID)
			require.Equal(t, tc.username, data.Username)
			require.Equal(t, tc.project, data.Meta.Project)
			require.Equal(t, tc.expectedRootNamespace, data.Meta.RootNamespace)
			require.Equal(t, tc.projectID, data.Meta.ProjectID)
			require.Equal(t, tc.rootNamespaceID, data.Meta.RootNamespaceID)
		})
	}
}

func TestCheckForVersionFlag(t *testing.T) {
	if os.Getenv("GITLAB_SHELL_TEST_CHECK_FOR_VERSION_FLAG") == "1" {
		CheckForVersionFlag([]string{"test", "-version"}, "1.2.3", "456")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCheckForVersionFlag")
	cmd.Env = append(os.Environ(), "GITLAB_SHELL_TEST_CHECK_FOR_VERSION_FLAG=1")
	out, err := cmd.Output()

	require.NoError(t, err)
	require.Equal(t, "test 1.2.3-456\n", string(out))
}

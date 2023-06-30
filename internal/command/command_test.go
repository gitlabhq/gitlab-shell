package command

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/labkit/correlation"
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
			resetEnvironment := addAdditionalEnv(tc.additionalEnv)
			defer resetEnvironment()

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

// addAdditionalEnv will configure additional environment values
// and return a deferrable function to reset the environment to
// it's original state after the test
func addAdditionalEnv(envMap map[string]string) func() {
	prevValues := map[string]string{}
	unsetValues := []string{}
	for k, v := range envMap {
		value, exists := os.LookupEnv(k)
		if exists {
			prevValues[k] = value
		} else {
			unsetValues = append(unsetValues, k)
		}
		os.Setenv(k, v)
	}

	return func() {
		for k, v := range prevValues {
			os.Setenv(k, v)
		}

		for _, k := range unsetValues {
			os.Unsetenv(k)
		}

	}
}

func TestNewLogMetadata(t *testing.T) {
	testCases := []struct {
		desc                  string
		project               string
		username              string
		expectedRootNamespace string
	}{
		{
			desc:                  "Project under single namespace",
			project:               "flightjs/Flight",
			username:              "@alex-doe",
			expectedRootNamespace: "flightjs",
		},
		{
			desc:                  "Project under single odd namespace",
			project:               "flightjs///Flight",
			username:              "@alex-doe",
			expectedRootNamespace: "flightjs",
		},
		{
			desc:                  "Project under deeper namespace",
			project:               "flightjs/one/Flight",
			username:              "@alex-doe",
			expectedRootNamespace: "flightjs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			metaData := NewLogMetadata(tc.project, tc.username)
			require.Equal(t, tc.project, metaData.Project)
			require.Equal(t, tc.username, metaData.Username)
			require.Equal(t, tc.expectedRootNamespace, metaData.RootNamespace)
		})
	}
}

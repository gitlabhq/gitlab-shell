package checker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/checker/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/checker/authorizedprincipals"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/executable/fallback"
)

func TestNew(t *testing.T) {
	testCases := []struct {
		desc         string
		executable   *executable.Executable
		config       *config.Config
		expectedType interface{}
	}{
		{
			desc:       "it returns AuthorizedKeys checker",
			executable: &executable.Executable{Name: "gitlab-shell-authorized-keys-check"},
			config: &config.Config{
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"gitlab-shell-authorized-keys-check"}},
			},
			expectedType: &authorizedkeys.Checker{},
		},
		{
			desc:       "it returns AuthorizedPrincipals checker",
			executable: &executable.Executable{Name: "gitlab-shell-authorized-principals-check"},
			config: &config.Config{
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"gitlab-shell-authorized-principals-check"}},
			},
			expectedType: &authorizedprincipals.Checker{},
		},
		{
			desc:       "it returns fallback executable",
			executable: &executable.Executable{Name: "gitlab-shell-authorized-keys-check"},
			config: &config.Config{
				Migration: config.MigrationConfig{Enabled: false},
			},
			expectedType: &fallback.Executable{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			checker := New(tc.executable, []string{"arg"}, tc.config, nil)

			require.IsType(t, tc.expectedType, checker)
		})
	}
}

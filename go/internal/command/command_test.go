package command

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/authorizedprincipals"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/fallback"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/receivepack"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/uploadarchive"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/uploadpack"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/testhelper"
)

func TestNew(t *testing.T) {
	testCases := []struct {
		desc         string
		executable   *executable.Executable
		config       *config.Config
		environment  map[string]string
		arguments    []string
		expectedType interface{}
	}{
		{
			desc:       "it returns a Discover command if the feature is enabled",
			executable: &executable.Executable{Name: executable.GitlabShell},
			config: &config.Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"discover"}},
			},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{},
			expectedType: &discover.Command{},
		},
		{
			desc:       "it returns a Fallback command no feature is enabled",
			executable: &executable.Executable{Name: executable.GitlabShell},
			config: &config.Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: config.MigrationConfig{Enabled: false},
			},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{},
			expectedType: &fallback.Command{},
		},
		{
			desc:       "it returns a TwoFactorRecover command if the feature is enabled",
			executable: &executable.Executable{Name: executable.GitlabShell},
			config: &config.Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"2fa_recovery_codes"}},
			},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "2fa_recovery_codes",
			},
			arguments:    []string{},
			expectedType: &twofactorrecover.Command{},
		},
		{
			desc:       "it returns an LfsAuthenticate command if the feature is enabled",
			executable: &executable.Executable{Name: executable.GitlabShell},
			config: &config.Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"git-lfs-authenticate"}},
			},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-lfs-authenticate",
			},
			arguments:    []string{},
			expectedType: &lfsauthenticate.Command{},
		},
		{
			desc:       "it returns a ReceivePack command if the feature is enabled",
			executable: &executable.Executable{Name: executable.GitlabShell},
			config: &config.Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"git-receive-pack"}},
			},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-receive-pack",
			},
			arguments:    []string{},
			expectedType: &receivepack.Command{},
		},
		{
			desc:       "it returns an UploadPack command if the feature is enabled",
			executable: &executable.Executable{Name: executable.GitlabShell},
			config: &config.Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"git-upload-pack"}},
			},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-upload-pack",
			},
			arguments:    []string{},
			expectedType: &uploadpack.Command{},
		},
		{
			desc:       "it returns an UploadArchive command if the feature is enabled",
			executable: &executable.Executable{Name: executable.GitlabShell},
			config: &config.Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"git-upload-archive"}},
			},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-upload-archive",
			},
			arguments:    []string{},
			expectedType: &uploadarchive.Command{},
		},
		{
			desc:       "it returns a AuthorizedKeys command if the feature is enabled",
			executable: &executable.Executable{Name: executable.AuthorizedKeysCheck},
			config: &config.Config{
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"gitlab-shell-authorized-keys-check"}},
			},
			environment:  map[string]string{},
			arguments:    []string{"git", "git", "key"},
			expectedType: &authorizedkeys.Command{},
		},
		{
			desc:       "it returns a AuthorizedPrincipals command if the feature is enabled",
			executable: &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			config: &config.Config{
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"gitlab-shell-authorized-principals-check"}},
			},
			environment:  map[string]string{},
			arguments:    []string{"key", "principal"},
			expectedType: &authorizedprincipals.Command{},
		},
		{
			desc:       "it returns a Fallback command if the feature is unimplemented",
			executable: &executable.Executable{Name: executable.GitlabShell},
			config: &config.Config{
				GitlabUrl: "http+unix://gitlab.socket",
				Migration: config.MigrationConfig{Enabled: true, Features: []string{"git-unimplemented-feature"}},
			},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-unimplemented-feature",
			},
			arguments:    []string{},
			expectedType: &fallback.Command{},
		},
		{
			desc:         "it returns a Fallback command if executable is unknown",
			executable:   &executable.Executable{Name: "unknown"},
			config:       &config.Config{},
			arguments:    []string{},
			expectedType: &fallback.Command{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			command, err := New(tc.executable, tc.arguments, tc.config, nil)

			require.NoError(t, err)
			require.IsType(t, tc.expectedType, command)
		})
	}
}

func TestFailingNew(t *testing.T) {
	t.Run("It returns an error parsing arguments failed", func(t *testing.T) {
		_, err := New(&executable.Executable{Name: executable.GitlabShell}, []string{}, &config.Config{}, nil)

		require.Error(t, err)
	})
}

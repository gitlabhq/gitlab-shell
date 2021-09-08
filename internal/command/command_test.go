package command

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/authorizedprincipals"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/healthcheck"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/personalaccesstoken"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/receivepack"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/twofactorverify"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/uploadarchive"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/uploadpack"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
	"gitlab.com/gitlab-org/labkit/correlation"
)

var (
	authorizedKeysExec       = &executable.Executable{Name: executable.AuthorizedKeysCheck, AcceptArgs: true}
	authorizedPrincipalsExec = &executable.Executable{Name: executable.AuthorizedPrincipalsCheck, AcceptArgs: true}
	checkExec                = &executable.Executable{Name: executable.Healthcheck, AcceptArgs: false}
	gitlabShellExec          = &executable.Executable{Name: executable.GitlabShell, AcceptArgs: true}

	basicConfig    = &config.Config{GitlabUrl: "http+unix://gitlab.socket"}
	advancedConfig = &config.Config{GitlabUrl: "http+unix://gitlab.socket", SslCertDir: "/tmp/certs"}
)

func buildEnv(command string) sshenv.Env {
	return sshenv.Env{
		IsSSHConnection: true,
		OriginalCommand: command,
	}
}

func TestNew(t *testing.T) {
	testCases := []struct {
		desc         string
		executable   *executable.Executable
		env          sshenv.Env
		arguments    []string
		config       *config.Config
		expectedType interface{}
	}{
		{
			desc:         "it returns a Discover command",
			executable:   gitlabShellExec,
			env:          buildEnv(""),
			config:       basicConfig,
			expectedType: &discover.Command{},
		},
		{
			desc:         "it returns a TwoFactorRecover command",
			executable:   gitlabShellExec,
			env:          buildEnv("2fa_recovery_codes"),
			config:       basicConfig,
			expectedType: &twofactorrecover.Command{},
		},
		{
			desc:         "it returns a TwoFactorVerify command",
			executable:   gitlabShellExec,
			env:          buildEnv("2fa_verify"),
			config:       basicConfig,
			expectedType: &twofactorverify.Command{},
		},
		{
			desc:         "it returns an LfsAuthenticate command",
			executable:   gitlabShellExec,
			env:          buildEnv("git-lfs-authenticate"),
			config:       basicConfig,
			expectedType: &lfsauthenticate.Command{},
		},
		{
			desc:         "it returns a ReceivePack command",
			executable:   gitlabShellExec,
			env:          buildEnv("git-receive-pack"),
			config:       basicConfig,
			expectedType: &receivepack.Command{},
		},
		{
			desc:         "it returns an UploadPack command",
			executable:   gitlabShellExec,
			env:          buildEnv("git-upload-pack"),
			config:       basicConfig,
			expectedType: &uploadpack.Command{},
		},
		{
			desc:         "it returns an UploadArchive command",
			executable:   gitlabShellExec,
			env:          buildEnv("git-upload-archive"),
			config:       basicConfig,
			expectedType: &uploadarchive.Command{},
		},
		{
			desc:         "it returns a Healthcheck command",
			executable:   checkExec,
			config:       basicConfig,
			expectedType: &healthcheck.Command{},
		},
		{
			desc:         "it returns a AuthorizedKeys command",
			executable:   authorizedKeysExec,
			arguments:    []string{"git", "git", "key"},
			config:       basicConfig,
			expectedType: &authorizedkeys.Command{},
		},
		{
			desc:         "it returns a AuthorizedPrincipals command",
			executable:   authorizedPrincipalsExec,
			arguments:    []string{"key", "principal"},
			config:       basicConfig,
			expectedType: &authorizedprincipals.Command{},
		},
		{
			desc:         "it returns a PersonalAccessToken command",
			executable:   gitlabShellExec,
			env:          buildEnv("personal_access_token"),
			config:       basicConfig,
			expectedType: &personalaccesstoken.Command{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			command, err := New(tc.executable, tc.arguments, tc.env, tc.config, nil)

			require.NoError(t, err)
			require.IsType(t, tc.expectedType, command)
		})
	}
}

func TestFailingNew(t *testing.T) {
	testCases := []struct {
		desc          string
		executable    *executable.Executable
		env           sshenv.Env
		expectedError error
	}{
		{
			desc:          "Parsing environment failed",
			executable:    gitlabShellExec,
			expectedError: errors.New("Only SSH allowed"),
		},
		{
			desc:          "Unknown command given",
			executable:    gitlabShellExec,
			env:           buildEnv("unknown"),
			expectedError: disallowedcommand.Error,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			command, err := New(tc.executable, []string{}, tc.env, basicConfig, nil)
			require.Nil(t, command)
			require.Equal(t, tc.expectedError, err)
		})
	}
}

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

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
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/uploadarchive"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/uploadpack"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
	"gitlab.com/gitlab-org/labkit/correlation"
)

var (
	authorizedKeysExec       = &executable.Executable{Name: executable.AuthorizedKeysCheck}
	authorizedPrincipalsExec = &executable.Executable{Name: executable.AuthorizedPrincipalsCheck}
	checkExec                = &executable.Executable{Name: executable.Healthcheck}
	gitlabShellExec          = &executable.Executable{Name: executable.GitlabShell}

	basicConfig = &config.Config{GitlabUrl: "http+unix://gitlab.socket"}
)

func buildEnv(command string) map[string]string {
	return map[string]string{
		"SSH_CONNECTION":       "1",
		"SSH_ORIGINAL_COMMAND": command,
	}
}

func TestNew(t *testing.T) {
	testCases := []struct {
		desc         string
		executable   *executable.Executable
		environment  map[string]string
		arguments    []string
		expectedType interface{}
	}{
		{
			desc:         "it returns a Discover command",
			executable:   gitlabShellExec,
			environment:  buildEnv(""),
			expectedType: &discover.Command{},
		},
		{
			desc:         "it returns a TwoFactorRecover command",
			executable:   gitlabShellExec,
			environment:  buildEnv("2fa_recovery_codes"),
			expectedType: &twofactorrecover.Command{},
		},
		{
			desc:         "it returns an LfsAuthenticate command",
			executable:   gitlabShellExec,
			environment:  buildEnv("git-lfs-authenticate"),
			expectedType: &lfsauthenticate.Command{},
		},
		{
			desc:         "it returns a ReceivePack command",
			executable:   gitlabShellExec,
			environment:  buildEnv("git-receive-pack"),
			expectedType: &receivepack.Command{},
		},
		{
			desc:         "it returns an UploadPack command",
			executable:   gitlabShellExec,
			environment:  buildEnv("git-upload-pack"),
			expectedType: &uploadpack.Command{},
		},
		{
			desc:         "it returns an UploadArchive command",
			executable:   gitlabShellExec,
			environment:  buildEnv("git-upload-archive"),
			expectedType: &uploadarchive.Command{},
		},
		{
			desc:         "it returns a Healthcheck command",
			executable:   checkExec,
			expectedType: &healthcheck.Command{},
		},
		{
			desc:         "it returns a AuthorizedKeys command",
			executable:   authorizedKeysExec,
			arguments:    []string{"git", "git", "key"},
			expectedType: &authorizedkeys.Command{},
		},
		{
			desc:         "it returns a AuthorizedPrincipals command",
			executable:   authorizedPrincipalsExec,
			arguments:    []string{"key", "principal"},
			expectedType: &authorizedprincipals.Command{},
		},
		{
			desc:         "it returns a PersonalAccessToken command",
			executable:   gitlabShellExec,
			environment:  buildEnv("personal_access_token"),
			expectedType: &personalaccesstoken.Command{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			command, err := New(tc.executable, tc.arguments, basicConfig, nil)

			require.NoError(t, err)
			require.IsType(t, tc.expectedType, command)
		})
	}
}

func TestFailingNew(t *testing.T) {
	testCases := []struct {
		desc          string
		executable    *executable.Executable
		environment   map[string]string
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
			environment:   buildEnv("unknown"),
			expectedError: disallowedcommand.Error,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			command, err := New(tc.executable, []string{}, basicConfig, nil)
			require.Nil(t, command)
			require.Equal(t, tc.expectedError, err)
		})
	}
}

func TestContextWithCorrelationID(t *testing.T) {
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

			ctx, finished := ContextWithCorrelationID()
			require.NotNil(t, ctx, "ctx is nil")
			require.NotNil(t, finished, "finished is nil")
			correlationID := correlation.ExtractFromContext(ctx)
			require.NotEmpty(t, correlationID)

			if tc.expectedCorrelationID != "" {
				require.Equal(t, tc.expectedCorrelationID, correlationID)
			}
			defer finished()
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

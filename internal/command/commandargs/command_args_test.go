package commandargs

import (
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"

	"github.com/stretchr/testify/require"
)

func TestParseSuccess(t *testing.T) {
	testCases := []struct {
		desc         string
		executable   *executable.Executable
		environment  map[string]string
		arguments    []string
		expectedArgs CommandArgs
	}{
		// Setting the used env variables for every case to ensure we're
		// not using anything set in the original env.
		{
			desc:       "It sets discover as the command when the command string was empty",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{}, CommandType: Discover},
		},
		{
			desc:       "It finds the key id in any passed arguments",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{"hello", "key-123"},
			expectedArgs: &Shell{Arguments: []string{"hello", "key-123"}, SshArgs: []string{}, CommandType: Discover, GitlabKeyId: "123"},
		}, {
			desc:       "It finds the username in any passed arguments",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{"hello", "username-jane-doe"},
			expectedArgs: &Shell{Arguments: []string{"hello", "username-jane-doe"}, SshArgs: []string{}, CommandType: Discover, GitlabUsername: "jane-doe"},
		}, {
			desc:       "It parses 2fa_recovery_codes command",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "2fa_recovery_codes",
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"2fa_recovery_codes"}, CommandType: TwoFactorRecover},
		}, {
			desc:       "It parses git-receive-pack command",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-receive-pack group/repo",
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc:       "It parses git-receive-pack command and a project with single quotes",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git receive-pack 'group/repo'",
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc:       `It parses "git receive-pack" command`,
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git receive-pack "group/repo"`,
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc:       `It parses a command followed by control characters`,
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git-receive-pack group/repo; any command`,
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc:       "It parses git-upload-pack command",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git upload-pack "group/repo"`,
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-upload-pack", "group/repo"}, CommandType: UploadPack},
		}, {
			desc:       "It parses git-upload-archive command",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-upload-archive 'group/repo'",
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-upload-archive", "group/repo"}, CommandType: UploadArchive},
		}, {
			desc:       "It parses git-lfs-authenticate command",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-lfs-authenticate 'group/repo' download",
			},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-lfs-authenticate", "group/repo", "download"}, CommandType: LfsAuthenticate},
		}, {
			desc:         "It parses authorized-keys command",
			executable:   &executable.Executable{Name: executable.AuthorizedKeysCheck},
			arguments:    []string{"git", "git", "key"},
			expectedArgs: &AuthorizedKeys{Arguments: []string{"git", "git", "key"}, ExpectedUser: "git", ActualUser: "git", Key: "key"},
		}, {
			desc:         "It parses authorized-principals command",
			executable:   &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:    []string{"key", "principal-1", "principal-2"},
			expectedArgs: &AuthorizedPrincipals{Arguments: []string{"key", "principal-1", "principal-2"}, KeyId: "key", Principals: []string{"principal-1", "principal-2"}},
		}, {
			desc:         "Unknown executable",
			executable:   &executable.Executable{Name: "unknown"},
			arguments:    []string{},
			expectedArgs: &GenericArgs{Arguments: []string{}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			result, err := Parse(tc.executable, tc.arguments)

			require.NoError(t, err)
			require.Equal(t, tc.expectedArgs, result)
		})
	}
}

func TestParseFailure(t *testing.T) {
	testCases := []struct {
		desc          string
		executable    *executable.Executable
		environment   map[string]string
		arguments     []string
		expectedError string
	}{
		{
			desc:          "It fails if SSH connection is not set",
			executable:    &executable.Executable{Name: executable.GitlabShell},
			arguments:     []string{},
			expectedError: "Only SSH allowed",
		},
		{
			desc:       "It fails if SSH command is invalid",
			executable: &executable.Executable{Name: executable.GitlabShell},
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git receive-pack "`,
			},
			arguments:     []string{},
			expectedError: "Invalid SSH command",
		},
		{
			desc:          "With not enough arguments for the AuthorizedKeysCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedKeysCheck},
			arguments:     []string{"user"},
			expectedError: "# Insufficient arguments. 1. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>",
		},
		{
			desc:          "With too many arguments for the AuthorizedKeysCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedKeysCheck},
			arguments:     []string{"user", "user", "key", "something-else"},
			expectedError: "# Insufficient arguments. 4. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>",
		},
		{
			desc:          "With missing username for the AuthorizedKeysCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedKeysCheck},
			arguments:     []string{"user", "", "key"},
			expectedError: "# No username provided",
		},
		{
			desc:          "With missing key for the AuthorizedKeysCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedKeysCheck},
			arguments:     []string{"user", "user", ""},
			expectedError: "# No key provided",
		},
		{
			desc:          "With not enough arguments for the AuthorizedPrincipalsCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:     []string{"key"},
			expectedError: "# Insufficient arguments. 1. Usage\n#\tgitlab-shell-authorized-principals-check <key-id> <principal1> [<principal2>...]",
		},
		{
			desc:          "With missing key_id for the AuthorizedPrincipalsCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:     []string{"", "principal"},
			expectedError: "# No key_id provided",
		},
		{
			desc:          "With blank principal for the AuthorizedPrincipalsCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:     []string{"key", "principal", ""},
			expectedError: "# An invalid principal was provided",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			_, err := Parse(tc.executable, tc.arguments)

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

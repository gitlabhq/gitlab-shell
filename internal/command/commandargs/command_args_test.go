package commandargs

import (
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"

	"github.com/stretchr/testify/require"
)

func TestParseSuccess(t *testing.T) {
	testCases := []struct {
		desc         string
		executable   *executable.Executable
		env          sshenv.Env
		arguments    []string
		expectedArgs CommandArgs
		expectError  bool
	}{
		{
			desc:         "It sets discover as the command when the command string was empty",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{}, CommandType: Discover, Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		}, {
			desc:         "It finds the key id in any passed arguments",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "key-123"},
			expectedArgs: &Shell{Arguments: []string{"hello", "key-123"}, SshArgs: []string{}, CommandType: Discover, GitlabKeyId: "123", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		}, {
			desc:         "It finds the key id only if the argument is of <key-id> format",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "username-key-123"},
			expectedArgs: &Shell{Arguments: []string{"hello", "username-key-123"}, SshArgs: []string{}, CommandType: Discover, GitlabUsername: "key-123", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		}, {
			desc:         "It finds the username in any passed arguments",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "username-jane-doe"},
			expectedArgs: &Shell{Arguments: []string{"hello", "username-jane-doe"}, SshArgs: []string{}, CommandType: Discover, GitlabUsername: "jane-doe", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		}, {
			desc:         "It parses 2fa_recovery_codes command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "2fa_recovery_codes"},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"2fa_recovery_codes"}, CommandType: TwoFactorRecover, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "2fa_recovery_codes"}},
		}, {
			desc:         "It parses git-receive-pack command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-receive-pack group/repo"},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-receive-pack group/repo"}},
		}, {
			desc:         "It parses git-receive-pack command and a project with single quotes",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-receive-pack 'group/repo'"},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-receive-pack 'group/repo'"}},
		}, {
			desc:         `It parses "git receive-pack" command`,
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: `git-receive-pack "group/repo"`},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: `git-receive-pack "group/repo"`}},
		}, {
			desc:         `It parses a command followed by control characters`,
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: `git-receive-pack group/repo; any command`},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: `git-receive-pack group/repo; any command`}},
		}, {
			desc:         "It parses git-upload-pack command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: `git upload-pack "group/repo"`},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-upload-pack", "group/repo"}, CommandType: UploadPack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: `git upload-pack "group/repo"`}},
		}, {
			desc:         "It parses git-upload-archive command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-upload-archive 'group/repo'"},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-upload-archive", "group/repo"}, CommandType: UploadArchive, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-upload-archive 'group/repo'"}},
		}, {
			desc:         "It parses git-lfs-authenticate command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-lfs-authenticate 'group/repo' download"},
			arguments:    []string{},
			expectedArgs: &Shell{Arguments: []string{}, SshArgs: []string{"git-lfs-authenticate", "group/repo", "download"}, CommandType: LfsAuthenticate, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-lfs-authenticate 'group/repo' download"}},
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
			desc:        "Unknown executable",
			executable:  &executable.Executable{Name: "unknown"},
			arguments:   []string{},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := Parse(tc.executable, tc.arguments, tc.env)

			if !tc.expectError {
				require.NoError(t, err)
				require.Equal(t, tc.expectedArgs, result)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestParseFailure(t *testing.T) {
	testCases := []struct {
		desc          string
		executable    *executable.Executable
		env           sshenv.Env
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
			desc:          "It fails if SSH command is invalid",
			executable:    &executable.Executable{Name: executable.GitlabShell},
			env:           sshenv.Env{IsSSHConnection: true, OriginalCommand: `git receive-pack "`},
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
			_, err := Parse(tc.executable, tc.arguments, tc.env)

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

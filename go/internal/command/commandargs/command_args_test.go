package commandargs

import (
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/testhelper"

	"github.com/stretchr/testify/require"
)

func TestParseSuccess(t *testing.T) {
	testCases := []struct {
		desc         string
		environment  map[string]string
		arguments    []string
		expectedArgs CommandArgs
	}{
		// Setting the used env variables for every case to ensure we're
		// not using anything set in the original env.
		{
			desc: "It sets discover as the command when the command string was empty",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{}, CommandType: Discover},
		},
		{
			desc: "It finds the key id in any passed arguments",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{string(GitlabShell), "hello", "key-123"},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell), "hello", "key-123"}}, SshArgs: []string{}, CommandType: Discover, GitlabKeyId: "123"},
		}, {
			desc: "It finds the username in any passed arguments",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{string(GitlabShell), "hello", "username-jane-doe"},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell), "hello", "username-jane-doe"}}, SshArgs: []string{}, CommandType: Discover, GitlabUsername: "jane-doe"},
		}, {
			desc: "It parses 2fa_recovery_codes command",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "2fa_recovery_codes",
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{"2fa_recovery_codes"}, CommandType: TwoFactorRecover},
		}, {
			desc: "It parses git-receive-pack command",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-receive-pack group/repo",
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc: "It parses git-receive-pack command and a project with single quotes",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git receive-pack 'group/repo'",
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc: `It parses "git receive-pack" command`,
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git receive-pack "group/repo"`,
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc: `It parses a command followed by control characters`,
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git-receive-pack group/repo; any command`,
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc: "It parses git-upload-pack command",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git upload-pack "group/repo"`,
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{"git-upload-pack", "group/repo"}, CommandType: UploadPack},
		}, {
			desc: "It parses git-upload-archive command",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-upload-archive 'group/repo'",
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{"git-upload-archive", "group/repo"}, CommandType: UploadArchive},
		}, {
			desc: "It parses git-lfs-authenticate command",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-lfs-authenticate 'group/repo' download",
			},
			arguments:    []string{string(GitlabShell)},
			expectedArgs: &Shell{BaseArgs: &BaseArgs{arguments: []string{string(GitlabShell)}}, SshArgs: []string{"git-lfs-authenticate", "group/repo", "download"}, CommandType: LfsAuthenticate},
		}, {
			desc:         "Unknown executable",
			arguments:    []string{"unknown"},
			expectedArgs: &BaseArgs{arguments: []string{"unknown"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			result, err := Parse(tc.arguments)

			require.NoError(t, err)
			require.Equal(t, tc.expectedArgs, result)
		})
	}
}

func TestParseFailure(t *testing.T) {
	testCases := []struct {
		desc          string
		environment   map[string]string
		arguments     []string
		expectedError string
	}{
		{
			desc:          "It fails if SSH connection is not set",
			arguments:     []string{string(GitlabShell)},
			expectedError: "Only ssh allowed",
		},
		{
			desc: "It fails if SSH command is invalid",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git receive-pack "`,
			},
			arguments:     []string{string(GitlabShell)},
			expectedError: "Only ssh allowed",
		},
		{
			desc:          "It fails if arguments is empty",
			arguments:     []string{},
			expectedError: "arguments should include the executable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			_, err := Parse(tc.arguments)

			require.Error(t, err, tc.expectedError)
		})
	}
}

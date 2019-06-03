package commandargs

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/testhelper"
)

func TestParseSuccess(t *testing.T) {
	testCases := []struct {
		desc         string
		arguments    []string
		environment  map[string]string
		expectedArgs *CommandArgs
	}{
		// Setting the used env variables for every case to ensure we're
		// not using anything set in the original env.
		{
			desc: "It sets discover as the command when the command string was empty",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			expectedArgs: &CommandArgs{SshArgs: []string{}, CommandType: Discover},
		},
		{
			desc: "It finds the key id in any passed arguments",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{"hello", "key-123"},
			expectedArgs: &CommandArgs{SshArgs: []string{}, CommandType: Discover, GitlabKeyId: "123"},
		}, {
			desc: "It finds the username in any passed arguments",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "",
			},
			arguments:    []string{"hello", "username-jane-doe"},
			expectedArgs: &CommandArgs{SshArgs: []string{}, CommandType: Discover, GitlabUsername: "jane-doe"},
		}, {
			desc: "It parses 2fa_recovery_codes command",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "2fa_recovery_codes",
			},
			expectedArgs: &CommandArgs{SshArgs: []string{"2fa_recovery_codes"}, CommandType: TwoFactorRecover},
		}, {
			desc: "It parses git-receive-pack command",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git-receive-pack group/repo",
			},
			expectedArgs: &CommandArgs{SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc: "It parses git-receive-pack command and a project with single quotes",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": "git receive-pack 'group/repo'",
			},
			expectedArgs: &CommandArgs{SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc: `It parses "git receive-pack" command`,
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git receive-pack "group/repo"`,
			},
			expectedArgs: &CommandArgs{SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc: `It parses a command followed by control characters`,
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git-receive-pack group/repo; any command`,
			},
			expectedArgs: &CommandArgs{SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: ReceivePack},
		}, {
			desc: "It parses git-upload-pack command",
			environment: map[string]string{
				"SSH_CONNECTION":       "1",
				"SSH_ORIGINAL_COMMAND": `git upload-pack "group/repo"`,
			},
			expectedArgs: &CommandArgs{SshArgs: []string{"git-upload-pack", "group/repo"}, CommandType: UploadPack},
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
	t.Run("It fails if SSH connection is not set", func(t *testing.T) {
		_, err := Parse([]string{})

		require.Error(t, err, "Only ssh allowed")
	})

	t.Run("It fails if SSH command is invalid", func(t *testing.T) {
		environment := map[string]string{
			"SSH_CONNECTION":       "1",
			"SSH_ORIGINAL_COMMAND": `git receive-pack "`,
		}
		restoreEnv := testhelper.TempEnv(environment)
		defer restoreEnv()

		_, err := Parse([]string{})

		require.Error(t, err, "Invalid ssh command")
	})
}

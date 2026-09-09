package commandargs

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology"
)

func TestUserArgs(t *testing.T) {
	tests := []struct {
		name     string
		shell    Shell
		expected topology.UserArgs
	}{
		{
			name: "all fields populated",
			shell: Shell{
				GitlabUsername:      testUsername,
				GitlabKeyID:         "123",
				GitlabKrb5Principal: testKrb5Principal,
			},
			expected: topology.UserArgs{
				Username:      testUsername,
				KeyID:         "123",
				Krb5Principal: testKrb5Principal,
			},
		},
		{
			name:     "only username",
			shell:    Shell{GitlabUsername: testUsername},
			expected: topology.UserArgs{Username: testUsername},
		},
		{
			name:     "only key ID",
			shell:    Shell{GitlabKeyID: "123"},
			expected: topology.UserArgs{KeyID: "123"},
		},
		{
			name:     "only krb5 principal",
			shell:    Shell{GitlabKrb5Principal: testKrb5Principal},
			expected: topology.UserArgs{Krb5Principal: testKrb5Principal},
		},
		{
			name:     "empty shell",
			shell:    Shell{},
			expected: topology.UserArgs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.shell.UserArgs())
		})
	}
}

func TestShellParse(t *testing.T) {
	tests := []struct {
		name                string
		shell               Shell
		expectedSSHArgs     []string
		expectedCommandType CommandType
		expectedKeyID       string
		expectedError       string
		errorIs             error
	}{
		{
			name: "valid SSH command",
			shell: Shell{
				Arguments: []string{testKeyIDArgument},
				Env: sshenv.Env{
					IsSSHConnection: true,
					OriginalCommand: "git-receive-pack 'group/project.git'",
				},
			},
			expectedSSHArgs:     []string{string(ReceivePack), testProject},
			expectedCommandType: ReceivePack,
			expectedKeyID:       "123",
		},
		{
			name:          "non-SSH connection",
			shell:         Shell{},
			expectedError: "Only SSH allowed",
			errorIs:       ErrOnlySSHAllowed,
		},
		{
			name: "invalid SSH command",
			shell: Shell{Env: sshenv.Env{
				IsSSHConnection: true,
				OriginalCommand: `git receive-pack "`,
			}},
			expectedError: "Invalid SSH command: invalid command line string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.shell.Parse()

			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
				if tt.errorIs != nil {
					require.ErrorIs(t, err, tt.errorIs)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedSSHArgs, tt.shell.SSHArgs)
			require.Equal(t, tt.expectedCommandType, tt.shell.CommandType)
			require.Equal(t, tt.expectedKeyID, tt.shell.GitlabKeyID)
		})
	}
}

func TestShellParseCommand(t *testing.T) {
	tests := []struct {
		name                string
		command             string
		expectedSSHArgs     []string
		expectedCommandType CommandType
		expectedError       bool
	}{
		{name: "empty command", command: "", expectedSSHArgs: []string{}, expectedCommandType: Discover},
		{name: "two-factor recovery", command: "2fa_recovery_codes", expectedSSHArgs: []string{"2fa_recovery_codes"}, expectedCommandType: TwoFactorRecover},
		{name: "two-factor verification", command: "2fa_verify", expectedSSHArgs: []string{"2fa_verify"}, expectedCommandType: TwoFactorVerify},
		{name: "LFS authentication", command: "git-lfs-authenticate group/project.git download", expectedSSHArgs: []string{"git-lfs-authenticate", testProject, "download"}, expectedCommandType: LfsAuthenticate},
		{name: "LFS transfer", command: "git-lfs-transfer group/project.git download", expectedSSHArgs: []string{"git-lfs-transfer", testProject, "download"}, expectedCommandType: LfsTransfer},
		{name: "receive pack", command: "git-receive-pack group/project.git", expectedSSHArgs: []string{string(ReceivePack), testProject}, expectedCommandType: ReceivePack},
		{name: "upload pack", command: "git-upload-pack group/project.git", expectedSSHArgs: []string{string(UploadPack), testProject}, expectedCommandType: UploadPack},
		{name: "upload archive", command: "git-upload-archive group/project.git", expectedSSHArgs: []string{"git-upload-archive", testProject}, expectedCommandType: UploadArchive},
		{name: "personal access token", command: "personal_access_token", expectedSSHArgs: []string{"personal_access_token"}, expectedCommandType: PersonalAccessToken},
		{name: "Git for Windows command", command: `git upload-pack "group/project with spaces.git"`, expectedSSHArgs: []string{string(UploadPack), testSpacedProject}, expectedCommandType: UploadPack},
		{name: "single-quoted argument", command: `git-receive-pack 'group/project with spaces.git'`, expectedSSHArgs: []string{string(ReceivePack), testSpacedProject}, expectedCommandType: ReceivePack},
		{name: "escaped spaces", command: `git-upload-pack group/project\ with\ spaces.git`, expectedSSHArgs: []string{string(UploadPack), testSpacedProject}, expectedCommandType: UploadPack},
		{name: "empty quoted argument", command: `git-receive-pack ""`, expectedSSHArgs: []string{string(ReceivePack), ""}, expectedCommandType: ReceivePack},
		{name: "normalization guard", command: gitCommand, expectedSSHArgs: []string{gitCommand}, expectedCommandType: CommandType(gitCommand)},
		{name: "unknown command", command: "custom-command argument", expectedSSHArgs: []string{"custom-command", "argument"}, expectedCommandType: CommandType("custom-command")},
		{name: "unterminated single quote", command: `git-receive-pack '`, expectedError: true},
		{name: "unterminated double quote", command: `git-receive-pack "`, expectedError: true},
		{name: "unterminated backtick", command: "git-receive-pack `", expectedError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell := Shell{}

			err := shell.ParseCommand(tt.command)
			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedSSHArgs, shell.SSHArgs)
			require.Equal(t, tt.expectedCommandType, shell.CommandType)
		})
	}
}

func TestShellParseWho(t *testing.T) {
	tests := []struct {
		name             string
		arguments        []string
		expectedKeyID    string
		expectedUsername string
	}{
		{name: "key ID", arguments: []string{testKeyIDArgument}, expectedKeyID: "123"},
		{name: "username", arguments: []string{testUsernameAlice}, expectedUsername: testAlice},
		{name: "key ID from shell command", arguments: []string{"/path/to/gitlab-shell -c key-456"}, expectedKeyID: "456"},
		{name: "username from shell command", arguments: []string{"/path/to/gitlab-shell -c username-alice"}, expectedUsername: testAlice},
		// This case intentionally verifies that no identity is extracted from invalid arguments.
		{name: "invalid identities", arguments: []string{"key-abc", "username-", "unrelated"}, expectedKeyID: "", expectedUsername: ""},
		{name: "identity after ignored argument", arguments: []string{"ignored", testKeyIDArgument}, expectedKeyID: "123"},
		// whoKeyRegex (key-<digits>) and whoUsernameRegex (username-<nonspace>) are mutually exclusive per argument, so these cases verify the loop's first-recognized-argument-wins break behavior.
		{name: "username before key ID", arguments: []string{testUsernameAlice, testKeyIDArgument}, expectedUsername: testAlice},
		{name: "key ID before username", arguments: []string{testKeyIDArgument, testUsernameAlice}, expectedKeyID: "123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell := Shell{
				Arguments: tt.arguments,
				Env:       sshenv.Env{IsSSHConnection: true},
			}

			require.NoError(t, shell.Parse())
			require.Equal(t, tt.expectedKeyID, shell.GitlabKeyID)
			require.Equal(t, tt.expectedUsername, shell.GitlabUsername)
		})
	}
}

func TestShellGetArguments(t *testing.T) {
	require.Equal(t, []string{testKeyIDArgument, testExtra}, (&Shell{Arguments: []string{testKeyIDArgument, testExtra}}).GetArguments())
	require.Nil(t, (&Shell{}).GetArguments())
}

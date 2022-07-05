package command_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/discover"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/lfsauthenticate"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/personalaccesstoken"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/receivepack"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/twofactorrecover"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/twofactorverify"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/uploadarchive"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/uploadpack"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

var (
	gitlabShellExec = &executable.Executable{Name: executable.GitlabShell}
	basicConfig     = &config.Config{GitlabUrl: "http+unix://gitlab.socket"}
)

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
			desc:         "it returns a PersonalAccessToken command",
			executable:   gitlabShellExec,
			env:          buildEnv("personal_access_token"),
			config:       basicConfig,
			expectedType: &personalaccesstoken.Command{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			command, err := cmd.New(tc.arguments, tc.env, tc.config, nil)

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
			command, err := cmd.New([]string{}, tc.env, basicConfig, nil)
			require.Nil(t, command)
			require.Equal(t, tc.expectedError, err)
		})
	}
}

func buildEnv(command string) sshenv.Env {
	return sshenv.Env{
		IsSSHConnection: true,
		OriginalCommand: command,
	}
}

func TestParseSuccess(t *testing.T) {
	testCases := []struct {
		desc         string
		executable   *executable.Executable
		env          sshenv.Env
		arguments    []string
		expectedArgs commandargs.CommandArgs
		expectError  bool
	}{
		{
			desc:         "It sets discover as the command when the command string was empty",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{}, CommandType: commandargs.Discover, Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		},
		{
			desc:         "It finds the key id in any passed arguments",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "key-123"},
			expectedArgs: &commandargs.Shell{Arguments: []string{"hello", "key-123"}, SshArgs: []string{}, CommandType: commandargs.Discover, GitlabKeyId: "123", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		},
		{
			desc:         "It finds the key id only if the argument is of <key-id> format",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "username-key-123"},
			expectedArgs: &commandargs.Shell{Arguments: []string{"hello", "username-key-123"}, SshArgs: []string{}, CommandType: commandargs.Discover, GitlabUsername: "key-123", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		},
		{
			desc:         "It finds the key id if the key is listed as the last argument",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "gitlab-shell -c key-123"},
			expectedArgs: &commandargs.Shell{Arguments: []string{"hello", "gitlab-shell -c key-123"}, SshArgs: []string{}, CommandType: commandargs.Discover, GitlabKeyId: "123", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		},
		{
			desc:         "It finds the username if the username is listed as the last argument",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "gitlab-shell -c username-jane-doe"},
			expectedArgs: &commandargs.Shell{Arguments: []string{"hello", "gitlab-shell -c username-jane-doe"}, SshArgs: []string{}, CommandType: commandargs.Discover, GitlabUsername: "jane-doe", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		},
		{
			desc:         "It finds the key id only if the last argument is of <key-id> format",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "gitlab-shell -c username-key-123"},
			expectedArgs: &commandargs.Shell{Arguments: []string{"hello", "gitlab-shell -c username-key-123"}, SshArgs: []string{}, CommandType: commandargs.Discover, GitlabUsername: "key-123", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		},
		{
			desc:         "It finds the username in any passed arguments",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"},
			arguments:    []string{"hello", "username-jane-doe"},
			expectedArgs: &commandargs.Shell{Arguments: []string{"hello", "username-jane-doe"}, SshArgs: []string{}, CommandType: commandargs.Discover, GitlabUsername: "jane-doe", Env: sshenv.Env{IsSSHConnection: true, RemoteAddr: "1"}},
		},
		{
			desc:         "It parses 2fa_recovery_codes command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "2fa_recovery_codes"},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{"2fa_recovery_codes"}, CommandType: commandargs.TwoFactorRecover, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "2fa_recovery_codes"}},
		},
		{
			desc:         "It parses git-receive-pack command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-receive-pack group/repo"},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: commandargs.ReceivePack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-receive-pack group/repo"}},
		},
		{
			desc:         "It parses git-receive-pack command and a project with single quotes",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-receive-pack 'group/repo'"},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: commandargs.ReceivePack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-receive-pack 'group/repo'"}},
		},
		{
			desc:         `It parses "git receive-pack" command`,
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: `git-receive-pack "group/repo"`},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: commandargs.ReceivePack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: `git-receive-pack "group/repo"`}},
		},
		{
			desc:         `It parses a command followed by control characters`,
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: `git-receive-pack group/repo; any command`},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{"git-receive-pack", "group/repo"}, CommandType: commandargs.ReceivePack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: `git-receive-pack group/repo; any command`}},
		},
		{
			desc:         "It parses git-upload-pack command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: `git upload-pack "group/repo"`},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{"git-upload-pack", "group/repo"}, CommandType: commandargs.UploadPack, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: `git upload-pack "group/repo"`}},
		},
		{
			desc:         "It parses git-upload-archive command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-upload-archive 'group/repo'"},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{"git-upload-archive", "group/repo"}, CommandType: commandargs.UploadArchive, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-upload-archive 'group/repo'"}},
		},
		{
			desc:         "It parses git-lfs-authenticate command",
			executable:   &executable.Executable{Name: executable.GitlabShell},
			env:          sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-lfs-authenticate 'group/repo' download"},
			arguments:    []string{},
			expectedArgs: &commandargs.Shell{Arguments: []string{}, SshArgs: []string{"git-lfs-authenticate", "group/repo", "download"}, CommandType: commandargs.LfsAuthenticate, Env: sshenv.Env{IsSSHConnection: true, OriginalCommand: "git-lfs-authenticate 'group/repo' download"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := cmd.Parse(tc.arguments, tc.env)

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
			expectedError: "Invalid SSH command: invalid command line string",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := cmd.Parse(tc.arguments, tc.env)

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

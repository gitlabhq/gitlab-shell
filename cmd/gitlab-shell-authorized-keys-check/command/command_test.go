package command_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell-authorized-keys-check/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

var (
	authorizedKeysExec = &executable.Executable{Name: executable.AuthorizedKeysCheck}
	basicConfig        = &config.Config{GitlabUrl: "http+unix://gitlab.socket"}
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
			desc:         "it returns a AuthorizedKeys command",
			executable:   authorizedKeysExec,
			arguments:    []string{"git", "git", "key"},
			config:       basicConfig,
			expectedType: &authorizedkeys.Command{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			command, err := command.New(tc.arguments, tc.config, nil)

			require.NoError(t, err)
			require.IsType(t, tc.expectedType, command)
		})
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
			desc:         "It parses authorized-keys command",
			executable:   &executable.Executable{Name: executable.AuthorizedKeysCheck},
			arguments:    []string{"git", "git", "key"},
			expectedArgs: &commandargs.AuthorizedKeys{Arguments: []string{"git", "git", "key"}, ExpectedUser: "git", ActualUser: "git", Key: "key"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := command.Parse(tc.arguments)

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
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := command.Parse(tc.arguments)

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

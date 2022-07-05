package command_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	cmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell-authorized-principals-check/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/authorizedprincipals"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

var (
	authorizedPrincipalsExec = &executable.Executable{Name: executable.AuthorizedPrincipalsCheck}
	basicConfig              = &config.Config{GitlabUrl: "http+unix://gitlab.socket"}
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
			desc:         "it returns a AuthorizedPrincipals command",
			executable:   authorizedPrincipalsExec,
			arguments:    []string{"key", "principal"},
			config:       basicConfig,
			expectedType: &authorizedprincipals.Command{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			command, err := cmd.New(tc.arguments, tc.config, nil)

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
			desc:         "It parses authorized-principals command",
			executable:   &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:    []string{"key", "principal-1", "principal-2"},
			expectedArgs: &commandargs.AuthorizedPrincipals{Arguments: []string{"key", "principal-1", "principal-2"}, KeyId: "key", Principals: []string{"principal-1", "principal-2"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := cmd.Parse(tc.arguments)

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
			_, err := cmd.Parse(tc.arguments)

			require.EqualError(t, err, tc.expectedError)
		})
	}
}

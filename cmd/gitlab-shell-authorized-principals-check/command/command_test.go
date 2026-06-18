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

const (
	testKeyID      = "key"
	testPrincipal1 = "principal-1"
	testPrincipal  = "principal"
	testPrincipal2 = "principal-2"
)

var (
	authorizedPrincipalsExec = &executable.Executable{Name: executable.AuthorizedPrincipalsCheck}
	basicConfig              = &config.Config{GitlabURL: "http+unix://gitlab.socket"}
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
			arguments:    []string{testKeyID, testPrincipal},
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
			desc:         "it parses authorized-principals command",
			executable:   &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:    []string{testKeyID, testPrincipal1, testPrincipal2},
			expectedArgs: &commandargs.AuthorizedPrincipals{Arguments: []string{testKeyID, testPrincipal1, testPrincipal2}, KeyID: testKeyID, Principals: []string{testPrincipal1, testPrincipal2}},
		},
		{
			desc:        "it fails when a principal is empty",
			executable:  &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:   []string{testKeyID, testPrincipal1, ""},
			expectError: true,
		},
		{
			desc:        "it fails when a key_id is empty",
			executable:  &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:   []string{"", testPrincipal},
			expectError: true,
		},
		{
			desc:        "it fails when not enough arguments are present",
			executable:  &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:   []string{testKeyID},
			expectError: true,
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
			arguments:     []string{testKeyID},
			expectedError: "# Insufficient arguments. 1. Usage\n#\tgitlab-shell-authorized-principals-check <key-id> <principal1> [<principal2>...]",
		},
		{
			desc:          "With missing key_id for the AuthorizedPrincipalsCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:     []string{"", testPrincipal},
			expectedError: "# No key_id provided",
		},
		{
			desc:          "With blank principal for the AuthorizedPrincipalsCheck",
			executable:    &executable.Executable{Name: executable.AuthorizedPrincipalsCheck},
			arguments:     []string{testKeyID, testPrincipal, ""},
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

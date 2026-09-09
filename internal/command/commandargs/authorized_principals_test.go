package commandargs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorizedPrincipalsParse(t *testing.T) {
	tests := []struct {
		name          string
		arguments     []string
		expected      AuthorizedPrincipals
		expectedError string
	}{
		{
			name:      "one principal",
			arguments: []string{testKeyIDArgument, testAlice},
			expected: AuthorizedPrincipals{
				KeyID:      testKeyIDArgument,
				Principals: []string{testAlice},
			},
		},
		{
			name:      "multiple principals",
			arguments: []string{testKeyIDArgument, testAlice, testBob},
			expected: AuthorizedPrincipals{
				KeyID:      testKeyIDArgument,
				Principals: []string{testAlice, testBob},
			},
		},
		{name: "no arguments", arguments: []string{}, expectedError: "# Insufficient arguments. 0. Usage\n#\tgitlab-shell-authorized-principals-check <key-id> <principal1> [<principal2>...]"},
		{name: "key ID only", arguments: []string{testKeyIDArgument}, expectedError: "# Insufficient arguments. 1. Usage\n#\tgitlab-shell-authorized-principals-check <key-id> <principal1> [<principal2>...]"},
		{name: "empty key ID", arguments: []string{"", testAlice}, expectedError: "# No key_id provided"},
		{name: "empty principal", arguments: []string{testKeyIDArgument, ""}, expectedError: "# An invalid principal was provided"},
		{name: "empty principal among valid principals", arguments: []string{testKeyIDArgument, testAlice, "", testBob}, expectedError: "# An invalid principal was provided"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizedPrincipals := AuthorizedPrincipals{Arguments: tt.arguments}

			err := authorizedPrincipals.Parse()
			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
				return
			}

			require.NoError(t, err)
			tt.expected.Arguments = tt.arguments
			require.Equal(t, tt.expected, authorizedPrincipals)
		})
	}
}

func TestAuthorizedPrincipalsGetArguments(t *testing.T) {
	require.Equal(t, []string{testKeyIDArgument, testSSHKey}, (&AuthorizedPrincipals{Arguments: []string{testKeyIDArgument, testSSHKey}}).GetArguments())
	require.Nil(t, (&AuthorizedPrincipals{}).GetArguments())
}

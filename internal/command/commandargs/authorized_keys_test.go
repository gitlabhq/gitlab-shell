package commandargs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorizedKeysParse(t *testing.T) {
	tests := []struct {
		name          string
		arguments     []string
		expected      AuthorizedKeys
		expectedError string
	}{
		{
			name:      "valid arguments",
			arguments: []string{testAlice, testBob, testSSHKey},
			expected: AuthorizedKeys{
				ExpectedUser: testAlice,
				ActualUser:   testBob,
				Key:          testSSHKey,
			},
		},
		{name: "no arguments", arguments: []string{}, expectedError: "# Insufficient arguments. 0. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>"},
		{name: "two arguments", arguments: []string{testAlice, testBob}, expectedError: "# Insufficient arguments. 2. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>"},
		{name: "four arguments", arguments: []string{testAlice, testBob, testKey, testExtra}, expectedError: "# Insufficient arguments. 4. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>"},
		{name: "empty expected username", arguments: []string{"", testAlice, testKey}, expectedError: "# No username provided"},
		{name: "empty actual username", arguments: []string{testAlice, "", testKey}, expectedError: "# No username provided"},
		{name: "empty key", arguments: []string{testAlice, testBob, ""}, expectedError: "# No key provided"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizedKeys := AuthorizedKeys{Arguments: tt.arguments}

			err := authorizedKeys.Parse()
			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
				return
			}

			require.NoError(t, err)
			tt.expected.Arguments = tt.arguments
			require.Equal(t, tt.expected, authorizedKeys)
		})
	}
}

func TestAuthorizedKeysGetArguments(t *testing.T) {
	require.Equal(t, []string{testAlice, testBob, testSSHKey}, (&AuthorizedKeys{Arguments: []string{testAlice, testBob, testSSHKey}}).GetArguments())
	require.Nil(t, (&AuthorizedKeys{}).GetArguments())
}

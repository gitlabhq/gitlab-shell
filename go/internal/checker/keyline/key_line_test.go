package keyline

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToString(t *testing.T) {
	keyLine := &KeyLine{
		Id:      "1",
		Value:   "public-key",
		Prefix:  "key",
		RootDir: "/tmp",
	}

	result, err := keyLine.ToString()

	require.NoError(t, err)
	require.Equal(t, `command="/tmp/bin/gitlab-shell key-1",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty public-key`, result)
}

func TestFailingToString(t *testing.T) {
	testCases := []struct {
		desc          string
		keyLine       *KeyLine
		expectedError string
	}{
		{
			desc:          "When Id has non-alphanumeric and non-dash characters in it",
			keyLine:       &KeyLine{Id: "key\n1"},
			expectedError: "Invalid key_id: key\n1",
		},
		{
			desc:          "When Value has newline in it",
			keyLine:       &KeyLine{Id: "key", Value: "public\nkey"},
			expectedError: "Invalid value: public\nkey",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := tc.keyLine.ToString()

			require.Empty(t, result)
			require.EqualError(t, err, tc.expectedError)
		})
	}
}

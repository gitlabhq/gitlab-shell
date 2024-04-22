package keyline

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func TestFailingNewPublicKeyLine(t *testing.T) {
	testCases := []struct {
		desc          string
		id            string
		publicKey     string
		expectedError string
	}{
		{
			desc:          "When Id has non-alphanumeric and non-dash characters in it",
			id:            "key\n1",
			publicKey:     "public-key",
			expectedError: "invalid key_id: key\n1",
		},
		{
			desc:          "When public key has newline in it",
			id:            "key",
			publicKey:     "public\nkey",
			expectedError: "invalid value: public\nkey",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := NewPublicKeyLine(tc.id, tc.publicKey, &config.Config{RootDir: "/tmp", SslCertDir: "/tmp/certs"})

			require.Empty(t, result)
			require.EqualError(t, err, tc.expectedError)
		})
	}
}

func TestFailingNewPrincipalKeyLine(t *testing.T) {
	testCases := []struct {
		desc          string
		keyID         string
		principal     string
		expectedError string
	}{
		{
			desc:          "When username has non-alphanumeric and non-dash characters in it",
			keyID:         "username\n1",
			principal:     "principal",
			expectedError: "invalid key_id: username\n1",
		},
		{
			desc:          "When principal has newline in it",
			keyID:         "username",
			principal:     "principal\n1",
			expectedError: "invalid value: principal\n1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := NewPrincipalKeyLine(tc.keyID, tc.principal, &config.Config{RootDir: "/tmp", SslCertDir: "/tmp/certs"})

			require.Empty(t, result)
			require.EqualError(t, err, tc.expectedError)
		})
	}
}

func TestToString(t *testing.T) {
	keyLine := &KeyLine{
		ID:     "1",
		Value:  "public-key",
		Prefix: "key",
		Config: &config.Config{RootDir: "/tmp"},
	}

	result := keyLine.ToString()
	require.Equal(t, `command="/tmp/bin/gitlab-shell key-1",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty public-key`, result)
}

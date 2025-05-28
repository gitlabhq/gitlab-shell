package keyline

import (
	"fmt"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/executable"
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
		{
			desc:          "When KeyID has an invalid character in it",
			keyID:         "user.name@domain",
			principal:     "principal1",
			expectedError: "invalid key_id: user.name@domain",
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

func TestSuccessfulNewPrincipalKeyLine(t *testing.T) {
	testCases := []struct {
		desc      string
		keyID     string
		principal string
	}{
		{
			desc:      "KeyID with dot",
			keyID:     "user.name",
			principal: "principal1",
		},
		{
			desc:      "KeyID with uppercase",
			keyID:     "UserName",
			principal: "principal1",
		},
		{
			desc:      "KeyID with dot and uppercase",
			keyID:     "User.Name.DEPARTMENT",
			principal: "principal1",
		},
		{
			desc:      "KeyID with hyphen, dot, uppercase, no space",
			keyID:     "User-name.Department_9",
			principal: "principal1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			currentConfig := &config.Config{RootDir: "/tmp", SslCertDir: "/tmp/certs"}
			keyLine, err := NewPrincipalKeyLine(tc.keyID, tc.principal, currentConfig)
			require.NoError(t, err)
			require.NotNil(t, keyLine)
			require.Equal(t, tc.keyID, keyLine.ID)
			require.Equal(t, tc.principal, keyLine.Value)
			require.Equal(t, PrincipalPrefix, keyLine.Prefix)

			// Optionally verify ToString output
			expectedCommand := fmt.Sprintf("%s %s-%s", path.Join(currentConfig.RootDir, executable.BinDir, executable.GitlabShell), PrincipalPrefix, tc.keyID)
			expectedOutput := fmt.Sprintf(`command="%s",%s %s`, expectedCommand, SSHOptions, tc.principal)
			require.Equal(t, expectedOutput, keyLine.ToString())
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

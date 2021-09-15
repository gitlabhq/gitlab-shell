package sshd

import (
	"context"
	"crypto/dsa"
	"crypto/rand"
	"crypto/rsa"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

func TestNewServerConfigWithoutHosts(t *testing.T) {
	_, err := newServerConfig(&config.Config{GitlabUrl: "http://localhost"})

	require.Error(t, err)
	require.Equal(t, "No host keys could be loaded, aborting", err.Error())
}

func TestFailedAuthorizedKeysClient(t *testing.T) {
	_, err := newServerConfig(&config.Config{GitlabUrl: "ftp://localhost"})

	require.Error(t, err)
	require.Equal(t, "failed to initialize GitLab client: Error creating http client: unknown GitLab URL prefix", err.Error())
}

func TestFailedGetAuthKey(t *testing.T) {
	testhelper.PrepareTestRootDir(t)

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testhelper.TestRoot, "certs/valid/server.key"),
			path.Join(testhelper.TestRoot, "certs/invalid-path.key"),
			path.Join(testhelper.TestRoot, "certs/invalid/server.crt"),
		},
	}

	cfg, err := newServerConfig(
		&config.Config{GitlabUrl: "http://localhost", User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	testCases := []struct {
		desc          string
		user          string
		key           ssh.PublicKey
		expectedError string
	}{
		{
			desc:          "wrong user",
			user:          "wrong-user",
			key:           rsaPublicKey(t),
			expectedError: "unknown user",
		}, {
			desc:          "prohibited dsa key",
			user:          "user",
			key:           dsaPublicKey(t),
			expectedError: "DSA is prohibited",
		}, {
			desc:          "API error",
			user:          "user",
			key:           rsaPublicKey(t),
			expectedError: "Internal API unreachable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err = cfg.getAuthKey(context.Background(), tc.user, tc.key)
			require.Error(t, err)
			require.Equal(t, tc.expectedError, err.Error())
		})
	}
}

func rsaPublicKey(t *testing.T) ssh.PublicKey {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	return publicKey
}

func dsaPublicKey(t *testing.T) ssh.PublicKey {
	privateKey := new(dsa.PrivateKey)
	params := new(dsa.Parameters)
	require.NoError(t, dsa.GenerateParameters(params, rand.Reader, dsa.L1024N160))

	privateKey.PublicKey.Parameters = *params
	require.NoError(t, dsa.GenerateKey(privateKey, rand.Reader))

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	return publicKey
}

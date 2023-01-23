package sshd

import (
	"context"
	"crypto/dsa"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

func TestNewServerConfigWithoutHosts(t *testing.T) {
	_, err := newServerConfig(&config.Config{GitlabUrl: "http://localhost"})

	require.Error(t, err)
	require.Equal(t, "No host keys could be loaded, aborting", err.Error())
}

func TestHostKeyAndCerts(t *testing.T) {
	testhelper.PrepareTestRootDir(t)

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testhelper.TestRoot, "certs/valid/server.key"),
		},
		HostCertFiles: []string{
			path.Join(testhelper.TestRoot, "certs/valid/server-cert.pub"),
			path.Join(testhelper.TestRoot, "certs/valid/server2-cert.pub"),
			path.Join(testhelper.TestRoot, "certs/invalid/server-cert.pub"),
			path.Join(testhelper.TestRoot, "certs/invalid-path.key"),
			path.Join(testhelper.TestRoot, "certs/invalid/server.crt"),
		},
	}

	cfg, err := newServerConfig(
		&config.Config{GitlabUrl: "http://localhost", User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	require.Len(t, cfg.hostKeys, 1)
	require.Len(t, cfg.hostKeyToCertMap, 1)

	// Check that the entry is pointing to the server's public key
	data, err := os.ReadFile(path.Join(testhelper.TestRoot, "certs/valid/server.pub"))
	require.NoError(t, err)

	publicKey, _, _, _, err := ssh.ParseAuthorizedKey(data)
	require.NoError(t, err)
	require.NotNil(t, publicKey)
	cert, ok := cfg.hostKeyToCertMap[string(publicKey.Marshal())]
	require.True(t, ok)
	require.NotNil(t, cert)
	require.Equal(t, cert, cfg.hostKeys[0].PublicKey())
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

func TestDefaultAlgorithms(t *testing.T) {
	srvCfg := &serverConfig{cfg: &config.Config{}}
	sshServerConfig := srvCfg.get(context.Background())

	require.Equal(t, supportedMACs, sshServerConfig.MACs)
	require.Equal(t, supportedKeyExchanges, sshServerConfig.KeyExchanges)
	require.Nil(t, sshServerConfig.Ciphers)

	sshServerConfig.SetDefaults()

	require.Equal(t, supportedMACs, sshServerConfig.MACs)
	require.Equal(t, supportedKeyExchanges, sshServerConfig.KeyExchanges)

	defaultCiphers := []string{
		"aes128-gcm@openssh.com",
		"chacha20-poly1305@openssh.com",
		"aes256-gcm@openssh.com",
		"aes128-ctr",
		"aes192-ctr",
		"aes256-ctr",
	}
	require.Equal(t, defaultCiphers, sshServerConfig.Ciphers)
}

func TestCustomAlgorithms(t *testing.T) {
	customMACs := []string{"hmac-sha2-512-etm@openssh.com"}
	customKexAlgos := []string{"curve25519-sha256"}
	customCiphers := []string{"aes256-gcm@openssh.com"}

	srvCfg := &serverConfig{
		cfg: &config.Config{
			Server: config.ServerConfig{
				MACs:          customMACs,
				KexAlgorithms: customKexAlgos,
				Ciphers:       customCiphers,
			},
		},
	}
	sshServerConfig := srvCfg.get(context.Background())

	require.Equal(t, customMACs, sshServerConfig.MACs)
	require.Equal(t, customKexAlgos, sshServerConfig.KeyExchanges)
	require.Equal(t, customCiphers, sshServerConfig.Ciphers)

	sshServerConfig.SetDefaults()

	require.Equal(t, customMACs, sshServerConfig.MACs)
	require.Equal(t, customKexAlgos, sshServerConfig.KeyExchanges)
	require.Equal(t, customCiphers, sshServerConfig.Ciphers)
}

func TestGSSAPIWithMIC(t *testing.T) {
	srvCfg := &serverConfig{
		cfg: &config.Config{
			Server: config.ServerConfig{
				GSSAPI: config.GSSAPIConfig{
					Enabled:              true,
					ServicePrincipalName: "host/test@TEST.TEST",
				},
			},
		},
	}
	sshServerConfig := srvCfg.get(context.Background())
	server := sshServerConfig.GSSAPIWithMICConfig.Server.(*OSGSSAPIServer)

	require.NotNil(t, sshServerConfig.GSSAPIWithMICConfig)
	require.NotNil(t, sshServerConfig.GSSAPIWithMICConfig.AllowLogin)
	require.NotNil(t, server)
	require.Equal(t, server.ServicePrincipalName, "host/test@TEST.TEST")

	sshServerConfig.SetDefaults()

	require.NotNil(t, sshServerConfig.GSSAPIWithMICConfig)
	require.NotNil(t, sshServerConfig.GSSAPIWithMICConfig.AllowLogin)
	require.NotNil(t, server)
	require.Equal(t, server.ServicePrincipalName, "host/test@TEST.TEST")
}

func TestGSSAPIWithMICDisabled(t *testing.T) {
	srvCfg := &serverConfig{
		cfg: &config.Config{
			Server: config.ServerConfig{
				GSSAPI: config.GSSAPIConfig{
					Enabled: false,
				},
			},
		},
	}
	sshServerConfig := srvCfg.get(context.Background())

	require.Nil(t, sshServerConfig.GSSAPIWithMICConfig)

	sshServerConfig.SetDefaults()

	require.Nil(t, sshServerConfig.GSSAPIWithMICConfig)
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

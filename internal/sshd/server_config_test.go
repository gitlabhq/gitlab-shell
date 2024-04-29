package sshd

import (
	"context"
	"crypto/dsa"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

func TestNewServerConfigWithoutHosts(t *testing.T) {
	_, err := newServerConfig(&config.Config{GitlabUrl: "http://localhost"})

	require.Error(t, err)
	require.Equal(t, "No host keys could be loaded, aborting", err.Error())
}

func TestHostKeyAndCerts(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testRoot, "certs/valid/server.key"),
		},
		HostCertFiles: []string{
			path.Join(testRoot, "certs/valid/server-cert.pub"),
			path.Join(testRoot, "certs/valid/server2-cert.pub"),
			path.Join(testRoot, "certs/invalid/server-cert.pub"),
			path.Join(testRoot, "certs/invalid-path.key"),
			path.Join(testRoot, "certs/invalid/server.crt"),
		},
	}

	cfg, err := newServerConfig(
		&config.Config{GitlabUrl: "http://localhost", User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	require.Len(t, cfg.hostKeys, 1)
	require.Len(t, cfg.hostKeyToCertMap, 1)

	// Check that the entry is pointing to the server's public key
	data, err := os.ReadFile(path.Join(testRoot, "certs/valid/server.pub"))
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
	require.Equal(t, "failed to initialize authorized keys client: error creating http client: unknown GitLab URL prefix", err.Error())
}

func TestUserKeyHandling(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	validRSAKey := rsaPublicKey(t)

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_keys",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				key := base64.RawStdEncoding.EncodeToString(validRSAKey.Marshal())
				if key == r.URL.Query().Get("key") {
					w.Write([]byte(`{ "id": 1, "key": "key" }`))
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
		},
	}

	url := testserver.StartSocketHttpServer(t, requests)

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testRoot, "certs/valid/server.key"),
			path.Join(testRoot, "certs/invalid-path.key"),
			path.Join(testRoot, "certs/invalid/server.crt"),
		},
	}

	cfg, err := newServerConfig(
		&config.Config{GitlabUrl: url, User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	testCases := []struct {
		desc                string
		user                string
		key                 ssh.PublicKey
		expectedErr         error
		expectedPermissions *ssh.Permissions
	}{
		{
			desc:        "wrong user",
			user:        "wrong-user",
			key:         rsaPublicKey(t),
			expectedErr: errors.New("unknown user"),
		}, {
			desc:        "prohibited dsa key",
			user:        "user",
			key:         dsaPublicKey(t),
			expectedErr: errors.New("DSA is prohibited"),
		}, {
			desc:        "API error",
			user:        "user",
			key:         rsaPublicKey(t),
			expectedErr: &client.APIError{Msg: "Internal API unreachable"},
		}, {
			desc: "successful request",
			user: "user",
			key:  validRSAKey,
			expectedPermissions: &ssh.Permissions{
				Extensions: map[string]string{"key-id": "1"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			permissions, err := cfg.handleUserKey(context.Background(), tc.user, tc.key)
			require.Equal(t, tc.expectedErr, err)
			require.Equal(t, tc.expectedPermissions, permissions)
		})
	}
}

func TestUserCertificateHandling(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	validUserCert := userCert(t, ssh.UserCert, time.Now().Add(time.Hour))

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_certs",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				key := strings.TrimPrefix(ssh.FingerprintSHA256(validUserCert.SignatureKey), "SHA256:")
				if key == r.URL.Query().Get("key") && r.URL.Query().Get("user_identifier") == "root@example.com" {
					w.Write([]byte(`{ "username": "root", "namespace": "namespace" }`))
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
			},
		},
	}

	url := testserver.StartSocketHttpServer(t, requests)

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testRoot, "certs/valid/server.key"),
			path.Join(testRoot, "certs/invalid-path.key"),
			path.Join(testRoot, "certs/invalid/server.crt"),
		},
	}

	cfg, err := newServerConfig(
		&config.Config{GitlabUrl: url, User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	testCases := []struct {
		desc                string
		cert                *ssh.Certificate
		featureFlagValue    string
		expectedErr         error
		expectedPermissions *ssh.Permissions
	}{
		{
			desc:             "wrong cert type",
			cert:             userCert(t, ssh.HostCert, time.Now().Add(time.Hour)),
			featureFlagValue: "1",
			expectedErr:      errors.New("handleUserCertificate: cert has type 2"),
		}, {
			desc:             "expired cert",
			cert:             userCert(t, ssh.UserCert, time.Now().Add(-time.Hour)),
			featureFlagValue: "1",
			expectedErr:      errors.New("ssh: cert has expired"),
		}, {
			desc:             "API error",
			cert:             userCert(t, ssh.UserCert, time.Now().Add(time.Hour)),
			featureFlagValue: "1",
			expectedErr:      &client.APIError{Msg: "Internal API unreachable"},
		}, {
			desc:             "successful request",
			cert:             validUserCert,
			featureFlagValue: "1",
			expectedPermissions: &ssh.Permissions{
				Extensions: map[string]string{
					"username":  "root",
					"namespace": "namespace",
				},
			},
		}, {
			desc:                "feature flag is not enabled",
			cert:                validUserCert,
			expectedErr:         errors.New("handleUserCertificate: feature is disabled"),
			expectedPermissions: nil,
		}, {
			desc:                "feature flag is disabled",
			cert:                validUserCert,
			featureFlagValue:    "0",
			expectedErr:         errors.New("handleUserCertificate: feature is disabled"),
			expectedPermissions: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Setenv("FF_GITLAB_SHELL_SSH_CERTIFICATES", tc.featureFlagValue)
			permissions, err := cfg.handleUserCertificate(context.Background(), "user", tc.cert)
			require.Equal(t, tc.expectedErr, err)
			require.Equal(t, tc.expectedPermissions, permissions)
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
		"aes256-gcm@openssh.com",
		"chacha20-poly1305@openssh.com",
		"aes128-ctr",
		"aes192-ctr",
		"aes256-ctr",
	}

	require.Equal(t, sshServerConfig.Ciphers, defaultCiphers)
}

func TestCustomAlgorithms(t *testing.T) {
	customMACs := []string{"hmac-sha2-512-etm@openssh.com"}
	customKexAlgos := []string{"curve25519-sha256"}
	customCiphers := []string{"aes256-gcm@openssh.com"}
	customPublicKeyAlgorithms := []string{"rsa-sha2-256"}

	srvCfg := &serverConfig{
		cfg: &config.Config{
			Server: config.ServerConfig{
				MACs:                customMACs,
				KexAlgorithms:       customKexAlgos,
				Ciphers:             customCiphers,
				PublicKeyAlgorithms: customPublicKeyAlgorithms,
			},
		},
	}
	sshServerConfig := srvCfg.get(context.Background())

	require.Equal(t, customMACs, sshServerConfig.MACs)
	require.Equal(t, customKexAlgos, sshServerConfig.KeyExchanges)
	require.Equal(t, customCiphers, sshServerConfig.Ciphers)
	require.Equal(t, customPublicKeyAlgorithms, sshServerConfig.PublicKeyAuthAlgorithms)

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

func userCert(t *testing.T, certType uint32, validBefore time.Time) *ssh.Certificate {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(privateKey)
	require.NoError(t, err)

	pubKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	cert := &ssh.Certificate{
		CertType:    certType,
		Key:         pubKey,
		KeyId:       "root@example.com",
		ValidBefore: uint64(validBefore.Unix()),
	}
	require.NoError(t, cert.SignCert(rand.Reader, signer))

	return cert
}

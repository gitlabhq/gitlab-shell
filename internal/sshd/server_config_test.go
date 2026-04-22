package sshd

import (
	"context"
	"crypto/dsa" //nolint:staticcheck // SA1019: Intentionally using deprecated DSA for testing rejection
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"log/slog"
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
	"gitlab.com/gitlab-org/labkit/fips"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

func TestNewServerConfigWithoutHosts(t *testing.T) {
	_, err := newServerConfig(&config.Config{GitlabURL: "http://localhost"})

	require.Error(t, err)
	require.Equal(t, "no host keys could be loaded, aborting", err.Error())
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
		&config.Config{GitlabURL: "http://localhost", User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	require.Len(t, cfg.hostKeys, 1)
	require.Len(t, cfg.hostKeyToCertMap, 1)

	// Check that the entry is pointing to the server's public key
	data, err := os.ReadFile(path.Join(testRoot, "certs/valid/server.pub"))
	require.NoError(t, err)

	publicKey, comment, _, _, err := ssh.ParseAuthorizedKey(data)
	require.NoError(t, err)
	require.NotNil(t, comment)
	require.NotNil(t, publicKey)
	cert, ok := cfg.hostKeyToCertMap[string(publicKey.Marshal())]
	require.True(t, ok)
	require.NotNil(t, cert)
	require.Equal(t, cert, cfg.hostKeys[0].PublicKey())
}

func TestNewServerConfigLoadsTrustedCAKeys(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	// Create a CA key file
	_, caPubKey := createCAKeyPair(t)
	caKeyFile := path.Join(testRoot, "ca.pub")
	err := os.WriteFile(caKeyFile, ssh.MarshalAuthorizedKey(caPubKey), 0600)
	require.NoError(t, err)

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testRoot, "certs/valid/server.key"),
		},
		TrustedUserCAKeys: []string{caKeyFile},
	}

	cfg, err := newServerConfig(
		&config.Config{GitlabURL: "http://localhost", User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	// Smoke test: verify CA keys were loaded via newServerConfig wiring
	require.Len(t, cfg.trustedUserCAKeySet, 1)
}

func TestNewServerConfig_FailsOnBadCAKeyFile(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testRoot, "certs/valid/server.key"),
		},
		TrustedUserCAKeys: []string{"/nonexistent/ca.pub"},
	}

	_, err := newServerConfig(
		&config.Config{GitlabURL: "http://localhost", User: "user", Server: srvCfg},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to load trusted user CA keys")
}

func TestFailedAuthorizedKeysClient(t *testing.T) {
	_, err := newServerConfig(&config.Config{GitlabURL: "ftp://localhost"})

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

	url := testserver.StartSocketHTTPServer(t, requests)

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
		&config.Config{GitlabURL: url, User: "user", Server: srvCfg},
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

	url := testserver.StartSocketHTTPServer(t, requests)

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
		&config.Config{GitlabURL: url, User: "user", Server: srvCfg},
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
			ctx := log.WithLogger(context.Background(), slog.Default())
			permissions, err := cfg.handleUserCertificate(ctx, "user", tc.cert)
			require.Equal(t, tc.expectedErr, err)
			require.Equal(t, tc.expectedPermissions, permissions)
		})
	}
}

func TestFipsDefaultAlgorithms(t *testing.T) {
	if !fips.Enabled() {
		t.Skip()
	}

	srvCfg := &serverConfig{cfg: &config.Config{}}
	sshServerConfig := srvCfg.get(context.Background())

	algorithms := fips.DefaultAlgorithms()

	require.Equal(t, algorithms.PublicKeyAuths, sshServerConfig.PublicKeyAuthAlgorithms)
	require.Equal(t, algorithms.MACs, sshServerConfig.MACs)
	require.Equal(t, algorithms.KeyExchanges, sshServerConfig.KeyExchanges)
	require.Equal(t, algorithms.Ciphers, sshServerConfig.Ciphers)
	// PublicKeyAuths is set at handshake time and by default includes ssh-rsa and ssh-dss
	require.Empty(t, algorithms.PublicKeyAuths)

	sshServerConfig.SetDefaults()

	// Go automatically adds curve25519-sha256@libssh.org as alias for curve25519-sha256
	// if the latter exists for backwards compatibility:
	// https://github.com/golang/crypto/blob/ef5341b70697ceb55f904384bd982587224e8b0c/ssh/common.go#L512-L520
	var kexs []string
	for _, k := range algorithms.KeyExchanges {
		kexs = append(kexs, k)
		if k == ssh.KeyExchangeCurve25519 {
			kexs = append(kexs, "curve25519-sha256@libssh.org")
		}
	}

	require.Equal(t, algorithms.MACs, sshServerConfig.MACs)
	require.Equal(t, kexs, sshServerConfig.KeyExchanges)
	require.Equal(t, algorithms.Ciphers, sshServerConfig.Ciphers)
}

func TestNonFipsDefaultAlgorithms(t *testing.T) {
	if fips.Enabled() {
		t.Skip()
	}

	srvCfg := &serverConfig{cfg: &config.Config{}}
	sshServerConfig := srvCfg.get(context.Background())

	defaultCfg := ssh.ServerConfig{}
	defaultCfg.SetDefaults()

	require.Equal(t, defaultCfg.PublicKeyAuthAlgorithms, sshServerConfig.PublicKeyAuthAlgorithms)
	require.Equal(t, defaultCfg.MACs, sshServerConfig.MACs)
	require.Equal(t, defaultCfg.KeyExchanges, sshServerConfig.KeyExchanges)
	require.Equal(t, defaultCfg.Ciphers, sshServerConfig.Ciphers)
}

func TestCustomAlgorithms(t *testing.T) {
	customMACs := []string{"hmac-sha2-512-etm@openssh.com"}
	customKexAlgos := []string{"curve25519-sha256", "curve25519-sha256@libssh.org"}
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
	require.Equal(t, "host/test@TEST.TEST", server.ServicePrincipalName)

	sshServerConfig.SetDefaults()

	require.NotNil(t, sshServerConfig.GSSAPIWithMICConfig)
	require.NotNil(t, sshServerConfig.GSSAPIWithMICConfig.AllowLogin)
	require.NotNil(t, server)
	require.Equal(t, "host/test@TEST.TEST", server.ServicePrincipalName)
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

//nolint:staticcheck // SA1019: Intentionally using deprecated DSA for testing rejection
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

func createCAKeyPair(t *testing.T) (ssh.Signer, ssh.PublicKey) {
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	caSigner, err := ssh.NewSignerFromKey(caPrivKey)
	require.NoError(t, err)

	caPubKey, err := ssh.NewPublicKey(&caPrivKey.PublicKey)
	require.NoError(t, err)

	return caSigner, caPubKey
}

func userCertSignedByCA(t *testing.T, caSigner ssh.Signer, certType uint32, validBefore time.Time, keyID string) *ssh.Certificate {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pubKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	cert := &ssh.Certificate{
		CertType:    certType,
		Key:         pubKey,
		KeyId:       keyID,
		ValidBefore: uint64(validBefore.Unix()),
	}
	require.NoError(t, cert.SignCert(rand.Reader, caSigner))

	return cert
}

func userCert(t *testing.T, certType uint32, validBefore time.Time) *ssh.Certificate {
	signer, _ := createCAKeyPair(t)
	return userCertSignedByCA(t, signer, certType, validBefore, "root@example.com")
}

func TestParseTrustedUserCAKeys(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	// Create a temporary CA key file
	_, caPubKey := createCAKeyPair(t)

	caKeyFile := path.Join(testRoot, "test_ca.pub")
	err := os.WriteFile(caKeyFile, ssh.MarshalAuthorizedKey(caPubKey), 0600)
	require.NoError(t, err)

	// Create a file with multiple CA keys
	_, caPubKey2 := createCAKeyPair(t)
	multiCAKeyFile := path.Join(testRoot, "multi_ca.pub")
	multiCAContent := append(ssh.MarshalAuthorizedKey(caPubKey), ssh.MarshalAuthorizedKey(caPubKey2)...)
	err = os.WriteFile(multiCAKeyFile, multiCAContent, 0600)
	require.NoError(t, err)

	// Create an invalid key file
	invalidKeyFile := path.Join(testRoot, "invalid_ca.pub")
	err = os.WriteFile(invalidKeyFile, []byte("not a valid ssh key"), 0600)
	require.NoError(t, err)

	// Create a file with valid key followed by invalid content (partial parse)
	partialCAKeyFile := path.Join(testRoot, "partial_ca.pub")
	partialContent := append(ssh.MarshalAuthorizedKey(caPubKey), []byte("invalid trailing content\n")...)
	err = os.WriteFile(partialCAKeyFile, partialContent, 0600)
	require.NoError(t, err)

	testCases := []struct {
		desc          string
		files         []string
		expectedCount int
		expectErr     bool
		errContains   string
	}{
		{
			desc:          "valid CA key file",
			files:         []string{caKeyFile},
			expectedCount: 1,
		},
		{
			desc:          "multiple CA keys in one file",
			files:         []string{multiCAKeyFile},
			expectedCount: 2,
		},
		{
			desc:          "multiple files with deduplication",
			files:         []string{caKeyFile, multiCAKeyFile},
			expectedCount: 2,
		},
		{
			desc:        "non-existent file",
			files:       []string{"/nonexistent/ca.pub"},
			expectErr:   true,
			errContains: "failed to read trusted user CA key file",
		},
		{
			desc:        "invalid key file",
			files:       []string{invalidKeyFile},
			expectErr:   true,
			errContains: "failed to parse trusted user CA key in file",
		},
		{
			desc:          "empty list",
			files:         []string{},
			expectedCount: 0,
		},
		{
			desc:        "mix of valid then invalid files",
			files:       []string{caKeyFile, invalidKeyFile},
			expectErr:   true,
			errContains: "failed to parse trusted user CA key in file",
		},
		{
			desc:        "partial parse - valid key followed by invalid content",
			files:       []string{partialCAKeyFile},
			expectErr:   true,
			errContains: "failed to parse trusted user CA key in file",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			keySet, err := parseTrustedUserCAKeys(tc.files)
			if tc.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
				require.Len(t, keySet, tc.expectedCount)
			}
		})
	}
}

func TestIsLocallyTrustedCA(t *testing.T) {
	_, caPubKey := createCAKeyPair(t)
	_, otherPubKey := createCAKeyPair(t)

	cfg := &serverConfig{
		trustedUserCAKeySet: map[string]struct{}{
			string(caPubKey.Marshal()): {},
		},
	}

	require.True(t, cfg.isLocallyTrustedCA(caPubKey))
	require.False(t, cfg.isLocallyTrustedCA(otherPubKey))

	// Test with empty trusted keys
	emptyCfg := &serverConfig{
		trustedUserCAKeySet: map[string]struct{}{},
	}
	require.False(t, emptyCfg.isLocallyTrustedCA(caPubKey))

	// Test with nil trusted keys
	nilCfg := &serverConfig{}
	require.False(t, nilCfg.isLocallyTrustedCA(caPubKey))
}

func TestValidateKeyID(t *testing.T) {
	testCases := []struct {
		desc      string
		keyID     string
		expectErr bool
		errMsg    string
	}{
		{desc: "valid simple username", keyID: "testuser", expectErr: false},
		{desc: "valid with dots", keyID: "jane.doe", expectErr: false},
		{desc: "valid with hyphens", keyID: "user-name", expectErr: false},
		{desc: "valid with underscores", keyID: "user_name", expectErr: false},
		{desc: "valid minimum length", keyID: "ab", expectErr: false},
		{desc: "valid maximum length", keyID: strings.Repeat("a", 255), expectErr: false},
		{desc: "valid mixed separators", keyID: "user.name-test_123", expectErr: false},
		{desc: "empty KeyId", keyID: "", expectErr: true, errMsg: "certificate has empty KeyId"},
		{desc: "single character", keyID: "a", expectErr: true, errMsg: "certificate KeyId length 1 is outside valid range [2, 255]"},
		{desc: "too long", keyID: strings.Repeat("a", 256), expectErr: true, errMsg: "certificate KeyId length 256 is outside valid range [2, 255]"},
		{desc: "starts with hyphen", keyID: "-username", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
		{desc: "ends with hyphen", keyID: "username-", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
		{desc: "starts with dot", keyID: ".username", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
		{desc: "ends with dot", keyID: "username.", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
		{desc: "consecutive dots", keyID: "user..name", expectErr: true, errMsg: "certificate KeyId contains consecutive special characters"},
		{desc: "consecutive hyphens", keyID: "user--name", expectErr: true, errMsg: "certificate KeyId contains consecutive special characters"},
		{desc: "consecutive mixed specials", keyID: "user.-name", expectErr: true, errMsg: "certificate KeyId contains consecutive special characters"},
		{desc: "contains newline", keyID: "user\nname", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
		{desc: "contains space", keyID: "user name", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
		{desc: "contains null byte", keyID: "user\x00name", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
		{desc: "contains at sign", keyID: "user@domain.com", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
		{desc: "non-ASCII characters", keyID: "пользователь", expectErr: true, errMsg: "certificate KeyId does not match GitLab username format"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := validateKeyID(tc.keyID)
			if tc.expectErr {
				require.Error(t, err)
				require.Equal(t, tc.errMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUserCertificateHandling_InstanceLevel(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	// Create a trusted CA key pair
	caSigner, caPubKey := createCAKeyPair(t)

	// Create an untrusted CA key pair
	untrustedSigner, _ := createCAKeyPair(t)

	// Create certificates
	validCert := userCertSignedByCA(t, caSigner, ssh.UserCert, time.Now().Add(time.Hour), "testuser")
	expiredCert := userCertSignedByCA(t, caSigner, ssh.UserCert, time.Now().Add(-time.Hour), "testuser")
	hostCert := userCertSignedByCA(t, caSigner, ssh.HostCert, time.Now().Add(time.Hour), "testuser")
	untrustedCert := userCertSignedByCA(t, untrustedSigner, ssh.UserCert, time.Now().Add(time.Hour), "testuser")
	emptyKeyIDCert := userCertSignedByCA(t, caSigner, ssh.UserCert, time.Now().Add(time.Hour), "")
	singleCharKeyIDCert := userCertSignedByCA(t, caSigner, ssh.UserCert, time.Now().Add(time.Hour), "a")
	newlineKeyIDCert := userCertSignedByCA(t, caSigner, ssh.UserCert, time.Now().Add(time.Hour), "user\nname")
	atSignKeyIDCert := userCertSignedByCA(t, caSigner, ssh.UserCert, time.Now().Add(time.Hour), "user@domain.com")
	dottedKeyIDCert := userCertSignedByCA(t, caSigner, ssh.UserCert, time.Now().Add(time.Hour), "jane.doe")
	consecutiveSpecialsCert := userCertSignedByCA(t, caSigner, ssh.UserCert, time.Now().Add(time.Hour), "user..name")

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testRoot, "certs/valid/server.key"),
		},
	}

	cfg, err := newServerConfig(
		&config.Config{GitlabURL: "http://localhost", User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	// Add the trusted CA
	cfg.trustedUserCAKeySet = map[string]struct{}{
		string(caPubKey.Marshal()): {},
	}

	testCases := []struct {
		desc                string
		cert                *ssh.Certificate
		expectedErr         string
		expectedPermissions *ssh.Permissions
	}{
		{
			desc: "valid instance-level certificate",
			cert: validCert,
			expectedPermissions: &ssh.Permissions{
				Extensions: map[string]string{
					"username": "testuser",
				},
			},
		},
		{
			desc: "valid instance-level certificate with dots in username",
			cert: dottedKeyIDCert,
			expectedPermissions: &ssh.Permissions{
				Extensions: map[string]string{
					"username": "jane.doe",
				},
			},
		},
		{
			desc:        "expired certificate",
			cert:        expiredCert,
			expectedErr: "ssh: cert has expired",
		},
		{
			desc:        "wrong cert type (host cert)",
			cert:        hostCert,
			expectedErr: "handleUserCertificate: cert has type 2",
		},
		{
			desc:        "untrusted CA without feature flag",
			cert:        untrustedCert,
			expectedErr: "handleUserCertificate: feature is disabled",
		},
		{
			desc:        "empty KeyId rejected",
			cert:        emptyKeyIDCert,
			expectedErr: "handleUserCertificate: certificate has empty KeyId",
		},
		{
			desc:        "single char KeyId rejected",
			cert:        singleCharKeyIDCert,
			expectedErr: "handleUserCertificate: certificate KeyId length 1 is outside valid range [2, 255]",
		},
		{
			desc:        "KeyId with newline rejected",
			cert:        newlineKeyIDCert,
			expectedErr: "handleUserCertificate: certificate KeyId does not match GitLab username format",
		},
		{
			desc:        "KeyId with at sign rejected",
			cert:        atSignKeyIDCert,
			expectedErr: "handleUserCertificate: certificate KeyId does not match GitLab username format",
		},
		{
			desc:        "KeyId with consecutive specials rejected",
			cert:        consecutiveSpecialsCert,
			expectedErr: "handleUserCertificate: certificate KeyId contains consecutive special characters",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx := log.WithLogger(context.Background(), slog.Default())
			permissions, err := cfg.handleUserCertificate(ctx, "user", tc.cert)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedPermissions, permissions)
		})
	}
}

func TestUserCertificateHandling_InstanceLevelWithMultipleCAs(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	// Create two trusted CA key pairs
	caSigner1, caPubKey1 := createCAKeyPair(t)
	caSigner2, caPubKey2 := createCAKeyPair(t)

	// Create certificates signed by different CAs
	certFromCA1 := userCertSignedByCA(t, caSigner1, ssh.UserCert, time.Now().Add(time.Hour), "user1")
	certFromCA2 := userCertSignedByCA(t, caSigner2, ssh.UserCert, time.Now().Add(time.Hour), "user2")

	srvCfg := config.ServerConfig{
		Listen:                  "127.0.0.1",
		ConcurrentSessionsLimit: 1,
		HostKeyFiles: []string{
			path.Join(testRoot, "certs/valid/server.key"),
		},
	}

	cfg, err := newServerConfig(
		&config.Config{GitlabURL: "http://localhost", User: "user", Server: srvCfg},
	)
	require.NoError(t, err)

	// Add both trusted CAs
	cfg.trustedUserCAKeySet = map[string]struct{}{
		string(caPubKey1.Marshal()): {},
		string(caPubKey2.Marshal()): {},
	}

	// Both certificates should be trusted
	ctx := log.WithLogger(context.Background(), slog.Default())
	permissions1, err := cfg.handleUserCertificate(ctx, "user", certFromCA1)
	require.NoError(t, err)
	require.Equal(t, &ssh.Permissions{
		Extensions: map[string]string{"username": "user1"},
	}, permissions1)

	permissions2, err := cfg.handleUserCertificate(ctx, "user", certFromCA2)
	require.NoError(t, err)
	require.Equal(t, &ssh.Permissions{
		Extensions: map[string]string{"username": "user2"},
	}, permissions2)
}

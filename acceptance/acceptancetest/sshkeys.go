//go:build acceptance

package acceptancetest

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// GenerateSSHKey returns a fresh ed25519 SSH signer.
func GenerateSSHKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromSigner(priv)
	require.NoError(t, err)
	return signer
}

// SignUserCert builds a user certificate for userKey, signed by ca, and returns
// a signer that an SSH client presents to authenticate. keyID becomes the
// certificate identity (sent to the API as user_identifier); principals are the
// SSH usernames the certificate is valid for.
func SignUserCert(t *testing.T, ca, userKey ssh.Signer, keyID string, principals []string, validity time.Duration) ssh.Signer {
	t.Helper()
	now := time.Now()
	cert := &ssh.Certificate{
		Key:             userKey.PublicKey(),
		CertType:        ssh.UserCert,
		KeyId:           keyID,
		ValidPrincipals: principals,
		ValidAfter:      uint64(now.Add(-time.Minute).Unix()),
		ValidBefore:     uint64(now.Add(validity).Unix()),
	}
	require.NoError(t, cert.SignCert(rand.Reader, ca))

	certSigner, err := ssh.NewCertSigner(cert, userKey)
	require.NoError(t, err)
	return certSigner
}

// CAFingerprint returns the CA key's SHA-256 fingerprint with the "SHA256:"
// prefix stripped — the exact value gitlab-sshd sends to the API as key=.
func CAFingerprint(ca ssh.Signer) string {
	return strings.TrimPrefix(ssh.FingerprintSHA256(ca.PublicKey()), "SHA256:")
}

// DialSSHCert dials addr and authenticates as user using signer. It returns nil
// when authentication succeeds and a non-nil error when it fails. The
// connection is closed immediately on success.
func DialSSHCert(addr, user string, signer ssh.Signer) error {
	clientCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	conn, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

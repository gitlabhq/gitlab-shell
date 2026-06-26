//go:build acceptance

package acceptancetest

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestSignUserCert(t *testing.T) {
	ca := GenerateSSHKey(t)
	userKey := GenerateSSHKey(t)

	signer := SignUserCert(t, ca, userKey, "testuser", []string{"git"}, time.Hour)

	cert, ok := signer.PublicKey().(*ssh.Certificate)
	require.True(t, ok, "signer public key must be a certificate")
	require.Equal(t, "testuser", cert.KeyId)
	require.Equal(t, []string{"git"}, cert.ValidPrincipals)
	require.Equal(t, uint32(ssh.UserCert), cert.CertType)

	// The fingerprint the daemon sends to the API is the CA key's SHA-256
	// fingerprint with the "SHA256:" prefix stripped.
	want := strings.TrimPrefix(ssh.FingerprintSHA256(ca.PublicKey()), "SHA256:")
	require.Equal(t, want, CAFingerprint(ca))
	require.Equal(t, want, strings.TrimPrefix(ssh.FingerprintSHA256(cert.SignatureKey), "SHA256:"))
}

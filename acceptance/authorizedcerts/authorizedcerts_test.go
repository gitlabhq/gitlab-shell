//go:build acceptance

package authorizedcerts_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/acceptance/acceptancetest"
)

const sshUser = "git"

// authGatedCertsEndpoint builds a fake /api/v4/internal/authorized_certs server.
// It 401s on an invalid JWT and otherwise responds with status/body. The
// inbound request is stored into *captured for assertions.
func authGatedCertsEndpoint(t *testing.T, secret string, status int, body string, captured **http.Request) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/internal/authorized_certs", func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Clone(r.Context())
		if !acceptancetest.JWTIsValid(r.Header.Get("Gitlab-Shell-Api-Request"), secret) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"bad auth"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	})
	return acceptancetest.NewFakeServer(t, mux)
}

func TestAuthorizedCerts_HappyPath(t *testing.T) {
	const secret = "test-secret"
	const keyID = "testuser"

	var captured *http.Request
	apiURL := authGatedCertsEndpoint(t, secret, http.StatusOK,
		`{"username":"alice","namespace":"alice-group"}`, &captured)

	ca := acceptancetest.GenerateSSHKey(t)
	userKey := acceptancetest.GenerateSSHKey(t)
	certSigner := acceptancetest.SignUserCert(t, ca, userKey, keyID, []string{sshUser}, time.Hour)

	d := acceptancetest.StartSSHD(t, acceptancetest.SSHDConfig{
		InternalAPIURL: apiURL,
		Secret:         secret,
		User:           sshUser,
		ExtraEnv:       map[string]string{"FF_GITLAB_SHELL_SSH_CERTIFICATES": "1"},
	})

	err := acceptancetest.DialSSHCert(d.Addr, sshUser, certSigner)
	require.NoError(t, err, "cert auth should succeed when the API authorizes the certificate")

	require.NotNil(t, captured, "the daemon should have called the authorized_certs API")
	require.Equal(t, "/api/v4/internal/authorized_certs", captured.URL.Path)
	require.Equal(t, acceptancetest.CAFingerprint(ca), captured.URL.Query().Get("key"))
	require.Equal(t, keyID, captured.URL.Query().Get("user_identifier"))
	require.Equal(t, "GitLab-Shell", captured.Header.Get("User-Agent"))
	require.True(t, acceptancetest.JWTIsValid(captured.Header.Get("Gitlab-Shell-Api-Request"), secret))
}

func TestAuthorizedCerts_Rejections(t *testing.T) {
	const secret = "test-secret"
	const keyID = "testuser"

	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"api_404", http.StatusNotFound, `{"message":"not found"}`},
		{"api_403", http.StatusForbidden, `{"message":"forbidden"}`},
		{"api_500", http.StatusInternalServerError, `boom`},
		{"api_empty_200", http.StatusOK, ``},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured *http.Request
			apiURL := authGatedCertsEndpoint(t, secret, tc.status, tc.body, &captured)

			ca := acceptancetest.GenerateSSHKey(t)
			userKey := acceptancetest.GenerateSSHKey(t)
			certSigner := acceptancetest.SignUserCert(t, ca, userKey, keyID, []string{sshUser}, time.Hour)

			d := acceptancetest.StartSSHD(t, acceptancetest.SSHDConfig{
				InternalAPIURL: apiURL,
				Secret:         secret,
				User:           sshUser,
				ExtraEnv:       map[string]string{"FF_GITLAB_SHELL_SSH_CERTIFICATES": "1"},
			})

			err := acceptancetest.DialSSHCert(d.Addr, sshUser, certSigner)
			require.Error(t, err, "cert auth must be rejected when the API does not authorize the certificate")
			require.NotNil(t, captured, "the daemon should still have called the API")
		})
	}
}

//go:build acceptance

package acceptancetest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// NewFakeServer starts an httptest.Server backed by mux, registers t.Cleanup to
// close it, and returns the base URL.
func NewFakeServer(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// JWTIsValid reports whether token is a well-formed HS256 JWT signed with
// secret, issued by "gitlab-shell", and unexpired.
func JWTIsValid(token, secret string) bool {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(strings.TrimSpace(secret)), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("gitlab-shell"))
	return err == nil && parsed.Valid
}

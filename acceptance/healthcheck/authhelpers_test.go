//go:build acceptance

package healthcheck_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// jwtIsValid returns true if token is a well-formed HS256 JWT signed
// with secret, has issuer "gitlab-shell", and is unexpired.
func jwtIsValid(token, secret string) bool {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(strings.TrimSpace(secret)), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("gitlab-shell"))
	return err == nil && parsed.Valid
}

// newFakeServer starts an httptest.Server backed by mux, registers t.Cleanup
// to close it, and returns the server's base URL.
func newFakeServer(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// authGatedHealthcheckEndpoint builds a fake /api/v4/internal/check server
// that 401s on invalid JWTs and 200s with body otherwise. It stores the
// incoming request into *captured for post-Run header assertions.
func authGatedHealthcheckEndpoint(t *testing.T, secret, body string, captured **http.Request) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/internal/check", func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Clone(r.Context())
		if !jwtIsValid(r.Header.Get("Gitlab-Shell-Api-Request"), secret) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, "bad auth")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	})
	return newFakeServer(t, mux)
}

// mintJWT is a test-only helper that produces a JWT in the same shape
// the production gitlab-shell clients produce: HS256, iss=gitlab-shell,
// short TTL. Used to drive jwtIsValid's positive cases.
func mintJWT(t *testing.T, secret string, ttl time.Duration, issuer string) string {
	t.Helper()
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)
	return tok
}

func TestJWTIsValid_acceptsCorrectToken(t *testing.T) {
	tok := mintJWT(t, "test-secret", time.Minute, "gitlab-shell")
	require.True(t, jwtIsValid(tok, "test-secret"))
}

func TestJWTIsValid_rejectsWrongSecret(t *testing.T) {
	tok := mintJWT(t, "test-secret", time.Minute, "gitlab-shell")
	require.False(t, jwtIsValid(tok, "different-secret"))
}

func TestJWTIsValid_rejectsWrongIssuer(t *testing.T) {
	tok := mintJWT(t, "test-secret", time.Minute, "not-gitlab-shell")
	require.False(t, jwtIsValid(tok, "test-secret"))
}

func TestJWTIsValid_rejectsExpiredToken(t *testing.T) {
	tok := mintJWT(t, "test-secret", -time.Minute, "gitlab-shell")
	require.False(t, jwtIsValid(tok, "test-secret"))
}

func TestJWTIsValid_rejectsEmptyToken(t *testing.T) {
	require.False(t, jwtIsValid("", "test-secret"))
}

func TestJWTIsValid_rejectsGarbage(t *testing.T) {
	require.False(t, jwtIsValid("not.a.jwt", "test-secret"))
}

//go:build acceptance

package healthcheck_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/elliotforbes/fakes"
	"github.com/gin-gonic/gin"
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

// authGatedHealthcheckEndpoint returns a fake endpoint for
// /api/v4/internal/check that 401s when the inbound JWT fails to verify
// against secret, and 200s with body otherwise. The handler stores the
// request into *captured so the caller can assert on headers after Run.
func authGatedHealthcheckEndpoint(secret, body string, captured **http.Request) *fakes.Endpoint {
	return &fakes.Endpoint{
		Path:    "/api/v4/internal/check",
		Methods: []string{http.MethodGet, http.MethodPost},
		Handler: func(c *gin.Context) {
			*captured = c.Request.Clone(c.Request.Context())
			if !jwtIsValid(c.Request.Header.Get("Gitlab-Shell-Api-Request"), secret) {
				c.String(http.StatusUnauthorized, "bad auth")
				return
			}
			c.Header("Content-Type", "application/json")
			c.String(http.StatusOK, body)
		},
	}
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

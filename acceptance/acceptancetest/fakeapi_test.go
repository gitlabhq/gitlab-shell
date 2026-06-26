//go:build acceptance

package acceptancetest

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestJWTIsValid(t *testing.T) {
	claims := jwt.RegisteredClaims{
		Issuer:    "gitlab-shell",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	require.NoError(t, err)

	require.True(t, JWTIsValid(tok, "test-secret"))
	require.False(t, JWTIsValid(tok, "wrong-secret"))
	require.False(t, JWTIsValid("", "test-secret"))
}

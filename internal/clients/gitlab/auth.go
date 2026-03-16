package gitlab

import (
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	apiSecretHeaderName = "Gitlab-Shell-Api-Request" // #nosec G101
	defaultUserAgent    = "GitLab-Shell"
	jwtTTL              = time.Minute
	jwtIssuer           = "gitlab-shell"
)

// jwtToken mints a short-lived HS256 token signed with the shared secret.
// A fresh token is generated per request because the TTL is only one minute;
// reusing a cached token across requests risks sending an expired credential
// if the caller batches requests or retries after a delay. This matches the
// behavior of client.GitlabNetClient.DoRequest.
func (c *Client) jwtToken() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(jwtTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(strings.TrimSpace(c.secret)))
}

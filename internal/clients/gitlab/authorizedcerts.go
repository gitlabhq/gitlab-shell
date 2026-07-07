package gitlab

import (
	"context"
	"net/url"
)

const authorizedCertsPath = "/authorized_certs"

// AuthorizedCertsResponse contains the authorized_certs endpoint response.
type AuthorizedCertsResponse struct {
	Username  string `json:"username"`
	Namespace string `json:"namespace"`
}

// AuthorizedCertsClient wraps the gitlab Client for authorized_certs requests.
type AuthorizedCertsClient struct {
	client *Client
}

// NewAuthorizedCertsClient creates a new authorized certs client.
func NewAuthorizedCertsClient(client *Client) *AuthorizedCertsClient {
	return &AuthorizedCertsClient{client: client}
}

// GetByKey requests authorized_certs for the given user identifier and signing
// CA fingerprint. The query parameters match the legacy gitlabnet client:
// key=<fingerprint> and user_identifier=<userID>.
func (c *AuthorizedCertsClient) GetByKey(ctx context.Context, userID, fingerprint string) (*AuthorizedCertsResponse, error) {
	path, err := authorizedCertsPathWithKey(userID, fingerprint)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var response AuthorizedCertsResponse
	if err := ParseJSON(resp, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func authorizedCertsPathWithKey(userID, fingerprint string) (string, error) {
	u, err := url.Parse(authorizedCertsPath)
	if err != nil {
		return "", err
	}
	params := u.Query()
	params.Set("key", fingerprint)
	params.Set("user_identifier", userID)
	u.RawQuery = params.Encode()
	return u.String(), nil
}

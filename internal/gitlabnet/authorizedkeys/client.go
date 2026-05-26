// Package authorizedkeys provides functionality for interacting with authorized keys.
package authorizedkeys

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"

	"gitlab.com/gitlab-org/labkit/v2/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology"
)

const (
	// AuthorizedKeysPath represents the path to authorized keys endpoint
	AuthorizedKeysPath = "/authorized_keys"
)

// Client represents a client for interacting with authorized keys
type Client struct {
	config   *config.Config
	client   *client.GitlabNetClient
	resolver *topology.Resolver
}

// Response represents the response structure for authorized keys
type Response struct {
	ID  int64  `json:"id"`
	Key string `json:"key"`
}

// NewClient creates a new instance of the authorized keys client
func NewClient(config *config.Config) (*Client, error) {
	client, err := gitlabnet.GetClient(config)
	if err != nil {
		return nil, fmt.Errorf("error creating http client: %v", err)
	}

	return &Client{
		config:   config,
		client:   client,
		resolver: config.NewTopologyResolver(),
	}, nil
}

// GetByKey retrieves authorized keys by key
func (c *Client) GetByKey(ctx context.Context, key string) (*Response, error) {
	path, err := pathWithKey(key)
	if err != nil {
		return nil, err
	}

	fingerprint, err := computeFingerprint(key)
	if err != nil {
		slog.DebugContext(ctx, "authorizedkeys: could not compute SSH key fingerprint for topology routing, falling back to default host",
			log.ErrorMessage(err.Error()))
	}
	routed := c.resolver.ClientForSSHFingerprint(ctx, c.client, fingerprint)

	response, err := routed.Client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	parsedResponse := &Response{}
	if err := gitlabnet.ParseJSON(response, parsedResponse); err != nil {
		return nil, err
	}

	return parsedResponse, nil
}

// computeFingerprint computes the SHA-256 fingerprint of an SSH key in the
// raw base64 format expected by the Topology Service (43 chars, no "SHA256:" prefix).
// The key parameter is the raw base64-encoded wire-format bytes of the public key
// (as produced by base64.RawStdEncoding.EncodeToString(key.Marshal())).
// Returns ("", nil) for an empty key. Returns an error if the key cannot be decoded.
func computeFingerprint(key string) (string, error) {
	if key == "" {
		return "", nil
	}
	keyBytes, err := base64.RawStdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("failed to decode key as base64: %w", err)
	}
	hash := sha256.Sum256(keyBytes)
	return base64.RawStdEncoding.EncodeToString(hash[:]), nil
}

func pathWithKey(key string) (string, error) {
	u, err := url.Parse(AuthorizedKeysPath)
	if err != nil {
		return "", err
	}

	params := u.Query()
	params.Set("key", key)
	u.RawQuery = params.Encode()

	return u.String(), nil
}

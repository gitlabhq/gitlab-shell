// Package gitlabnet provides client utilities for interacting with GitLab's internal API.
package gitlabnet

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// GetClient creates and returns a new GitlabNetClient configured with the provided settings.
func GetClient(config *config.Config) (*client.GitlabNetClient, error) {
	httpClient, err := config.HTTPClient()
	if err != nil {
		return nil, err
	}

	if httpClient == nil {
		return nil, fmt.Errorf("unsupported protocol")
	}

	return client.NewGitlabNetClient(config.HTTPSettings.User, config.HTTPSettings.Password, config.Secret, httpClient)
}

// ParseJSON decodes JSON from an HTTP response into the provided response interface.
func ParseJSON(hr *http.Response, response interface{}) error {
	if err := json.NewDecoder(hr.Body).Decode(response); err != nil {
		return fmt.Errorf("parsing failed")
	}

	return nil
}

// GlID represents a parsed GitLab gl_id identifier.
// Use UserID() or DeployTokenID() to extract the typed value.
type GlID struct {
	userID        int
	deployTokenID int
}

// UserID returns the numeric user ID and true if the gl_id represents a user.
func (g *GlID) UserID() (int, bool) {
	return g.userID, g.userID != 0
}

// DeployTokenID returns the numeric deploy token ID and true if the gl_id
// represents a deploy token.
func (g *GlID) DeployTokenID() (int, bool) {
	return g.deployTokenID, g.deployTokenID != 0
}

// ParseGlID parses a GitLab gl_id string into its typed representation.
//
// Known gl_id formats (see https://gitlab.com/gitlab-org/gitlab/-/blob/master/lib/gitlab/gl_id.rb):
//   - "user-<N>"         → UserID() returns (N, true)
//   - "deploy-token-<N>" → DeployTokenID() returns (N, true)
//
// Returns an error for unrecognized or malformed formats.
func ParseGlID(glID string) (*GlID, error) {
	switch {
	case strings.HasPrefix(glID, "user-"):
		id, err := strconv.Atoi(glID[len("user-"):])
		if err != nil {
			return nil, fmt.Errorf("invalid user gl_id %q: %w", glID, err)
		}
		return &GlID{userID: id}, nil
	case strings.HasPrefix(glID, "deploy-token-"):
		id, err := strconv.Atoi(glID[len("deploy-token-"):])
		if err != nil {
			return nil, fmt.Errorf("invalid deploy-token gl_id %q: %w", glID, err)
		}
		return &GlID{deployTokenID: id}, nil
	default:
		return nil, fmt.Errorf("unknown gl_id format %q", glID)
	}
}

// ParseIP extracts and returns the IP address from a remote address string.
// It handles both plain IP addresses and host:port combinations.
func ParseIP(remoteAddr string) string {
	// The remoteAddr field can be filled by:
	// 1. An IP address via the SSH_CONNECTION environment variable
	// 2. A host:port combination via the PROXY protocol
	ip, _, err := net.SplitHostPort(remoteAddr)

	// If we don't have a port or can't parse this address for some reason,
	// just return the original string.
	if err != nil {
		return remoteAddr
	}

	return ip
}

// Package gitlabnet provides client utilities for interacting with GitLab's internal API.
package gitlabnet

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// GetClient creates and returns a new GitlabNetClient configured with the provided settings.
func GetClient(config *config.Config) (*client.GitlabNetClient, error) {
	return NewGitLabClientFromConfig(config)
}

// NewGitLabClientFromConfig - this is a temporary constructor, it's purpose is to help facilitate
// the
func NewGitLabClientFromConfig(cfg *config.Config) (*client.GitlabNetClient, error) {
	httpClient, err := client.NewHTTPClientWithOpts(cfg.GitlabURL, cfg.GitlabRelativeURLRoot, cfg.HTTPSettings.CaFile, cfg.HTTPSettings.CaPath, cfg.HTTPSettings.ReadTimeoutSeconds, nil)
	if err != nil {
		return nil, err
	}

	gitlabClient, err := client.NewGitlabNetClient(cfg.HTTPSettings.User, cfg.HTTPSettings.Password, cfg.Secret, httpClient)
	if err != nil {
		return nil, err
	}
	return gitlabClient, nil
}

// ParseJSON decodes JSON from an HTTP response into the provided response interface.
func ParseJSON(hr *http.Response, response interface{}) error {
	if err := json.NewDecoder(hr.Body).Decode(response); err != nil {
		return fmt.Errorf("parsing failed")
	}

	return nil
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

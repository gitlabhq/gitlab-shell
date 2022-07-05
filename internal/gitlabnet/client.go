package gitlabnet

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

var (
	ParsingError = fmt.Errorf("Parsing failed")
)

func GetClient(config *config.Config) (*client.GitlabNetClient, error) {
	httpClient, err := config.HttpClient()
	if err != nil {
		return nil, err
	}

	if httpClient == nil {
		return nil, fmt.Errorf("Unsupported protocol")
	}

	return client.NewGitlabNetClient(config.HttpSettings.User, config.HttpSettings.Password, config.Secret, httpClient)
}

func ParseJSON(hr *http.Response, response interface{}) error {
	if err := json.NewDecoder(hr.Body).Decode(response); err != nil {
		return ParsingError
	}

	return nil
}

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

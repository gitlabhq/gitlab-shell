package gitlabnet

import (
	"encoding/json"
	"fmt"
	"net/http"

	"gitlab.com/gitlab-org/gitlab-shell/client"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)

var (
	ParsingError = fmt.Errorf("Parsing failed")
)

func GetClient(config *config.Config) (*client.GitlabNetClient, error) {
	httpClient := config.GetHttpClient()

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

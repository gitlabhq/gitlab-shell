package gitlabnet

import (
	"encoding/json"
	"fmt"
	"net/http"

	gitnet "gitlab.com/gitlab-org/gitaly/v14/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)

var (
	ParsingError = fmt.Errorf("Parsing failed")
)

func GetClient(config *config.Config) (*gitnet.GitlabNetClient, error) {
	httpClient, err := config.HttpClient()
	if err != nil {
		return nil, err
	}

	if httpClient == nil {
		return nil, fmt.Errorf("Unsupported protocol")
	}

	return gitnet.NewGitlabNetClient(config.HttpSettings.User, config.HttpSettings.Password, config.Secret, httpClient)
}

func ParseJSON(hr *http.Response, response interface{}) error {
	if err := json.NewDecoder(hr.Body).Decode(response); err != nil {
		return ParsingError
	}

	return nil
}

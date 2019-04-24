package gitlabnet

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

const (
	internalApiPath  = "/api/v4/internal"
	secretHeaderName = "Gitlab-Shared-Secret"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

type GitlabClient struct {
	httpClient *http.Client
	config     *config.Config
	host       string
}

func GetClient(config *config.Config) (*GitlabClient, error) {
	client := config.GetHttpClient()

	if client == nil {
		return nil, fmt.Errorf("Unsupported protocol")
	}

	return &GitlabClient{httpClient: client.HttpClient, config: config, host: client.Host}, nil
}

func normalizePath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	if !strings.HasPrefix(path, internalApiPath) {
		path = internalApiPath + path
	}
	return path
}

func newRequest(method, host, path string, data interface{}) (*http.Request, error) {
	path = normalizePath(path)

	var jsonReader io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		jsonReader = bytes.NewReader(jsonData)
	}

	request, err := http.NewRequest(method, host+path, jsonReader)
	if err != nil {
		return nil, err
	}

	return request, nil
}

func parseError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	defer resp.Body.Close()
	parsedResponse := &ErrorResponse{}

	if err := json.NewDecoder(resp.Body).Decode(parsedResponse); err != nil {
		return fmt.Errorf("Internal API error (%v)", resp.StatusCode)
	} else {
		return fmt.Errorf(parsedResponse.Message)
	}

}

func (c *GitlabClient) Get(path string) (*http.Response, error) {
	return c.doRequest("GET", path, nil)
}

func (c *GitlabClient) Post(path string, data interface{}) (*http.Response, error) {
	return c.doRequest("POST", path, data)
}

func (c *GitlabClient) doRequest(method, path string, data interface{}) (*http.Response, error) {
	request, err := newRequest(method, c.host, path, data)
	if err != nil {
		return nil, err
	}

	user, password := c.config.HttpSettings.User, c.config.HttpSettings.Password
	if user != "" && password != "" {
		request.SetBasicAuth(user, password)
	}

	encodedSecret := base64.StdEncoding.EncodeToString([]byte(c.config.Secret))
	request.Header.Set(secretHeaderName, encodedSecret)

	request.Header.Add("Content-Type", "application/json")
	request.Close = true

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Internal API unreachable")
	}

	if err := parseError(response); err != nil {
		return nil, err
	}

	return response, nil
}

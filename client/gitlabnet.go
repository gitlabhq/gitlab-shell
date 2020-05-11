package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	internalApiPath  = "/api/v4/internal"
	secretHeaderName = "Gitlab-Shared-Secret"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

type GitlabNetClient struct {
	httpClient             *HttpClient
	user, password, secret string
}

func NewGitlabNetClient(
	user,
	password,
	secret string,
	httpClient *HttpClient,
) (*GitlabNetClient, error) {

	if httpClient == nil {
		return nil, fmt.Errorf("Unsupported protocol")
	}

	return &GitlabNetClient{
		httpClient: httpClient,
		user:       user,
		password:   password,
		secret:     secret,
	}, nil
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
	if resp.StatusCode >= 200 && resp.StatusCode <= 399 {
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

func (c *GitlabNetClient) Get(path string) (*http.Response, error) {
	return c.DoRequest(http.MethodGet, normalizePath(path), nil)
}

func (c *GitlabNetClient) Post(path string, data interface{}) (*http.Response, error) {
	return c.DoRequest(http.MethodPost, normalizePath(path), data)
}

func (c *GitlabNetClient) DoRequest(method, path string, data interface{}) (*http.Response, error) {
	request, err := newRequest(method, c.httpClient.Host, path, data)
	if err != nil {
		return nil, err
	}

	user, password := c.user, c.password
	if user != "" && password != "" {
		request.SetBasicAuth(user, password)
	}

	encodedSecret := base64.StdEncoding.EncodeToString([]byte(c.secret))
	request.Header.Set(secretHeaderName, encodedSecret)

	request.Header.Add("Content-Type", "application/json")
	request.Close = true

	start := time.Now()
	response, err := c.httpClient.Do(request)
	fields := log.Fields{
		"method":      method,
		"url":         request.URL.String(),
		"duration_ms": time.Since(start) / time.Millisecond,
	}
	logger := log.WithFields(fields)

	if err != nil {
		logger.WithError(err).Error("Internal API unreachable")
		return nil, fmt.Errorf("Internal API unreachable")
	}

	if response != nil {
		logger = logger.WithField("status", response.StatusCode)
	}
	if err := parseError(response); err != nil {
		logger.WithError(err).Error("Internal API error")
		return nil, err
	}

	logger.Info("Finished HTTP request")

	return response, nil
}

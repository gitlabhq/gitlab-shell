package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/hashicorp/go-retryablehttp"

	"gitlab.com/gitlab-org/labkit/log"
)

const (
	internalApiPath     = "/api/v4/internal"
	secretHeaderName    = "Gitlab-Shared-Secret"
	apiSecretHeaderName = "Gitlab-Shell-Api-Request"
	defaultUserAgent    = "GitLab-Shell"
	jwtTTL              = time.Minute
	jwtIssuer           = "gitlab-shell"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

type GitlabNetClient struct {
	httpClient *HttpClient
	user       string
	password   string
	secret     string
	userAgent  string
}

type ApiError struct {
	Msg string
}

// To use as the key in a Context to set an X-Forwarded-For header in a request
type OriginalRemoteIPContextKey struct{}

func (e *ApiError) Error() string {
	return e.Msg
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
		userAgent:  defaultUserAgent,
	}, nil
}

// SetUserAgent overrides the default user agent for the User-Agent header field
// for subsequent requests for the GitlabNetClient
func (c *GitlabNetClient) SetUserAgent(ua string) {
	c.userAgent = ua
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

func appendPath(host string, path string) string {
	return strings.TrimSuffix(host, "/") + "/" + strings.TrimPrefix(path, "/")
}

func newRequest(ctx context.Context, method, host, path string, data interface{}) (*http.Request, error) {
	var jsonReader io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		jsonReader = bytes.NewReader(jsonData)
	}

	request, err := http.NewRequestWithContext(ctx, method, appendPath(host, path), jsonReader)
	if err != nil {
		return nil, err
	}

	return request, nil
}

func newRetryableRequest(ctx context.Context, method, host, path string, data interface{}) (*retryablehttp.Request, error) {
	var jsonReader io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		jsonReader = bytes.NewReader(jsonData)
	}

	request, err := retryablehttp.NewRequestWithContext(ctx, method, appendPath(host, path), jsonReader)
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
		return &ApiError{fmt.Sprintf("Internal API error (%v)", resp.StatusCode)}
	} else {
		return &ApiError{parsedResponse.Message}
	}

}

func (c *GitlabNetClient) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.DoRequest(ctx, http.MethodGet, normalizePath(path), nil)
}

func (c *GitlabNetClient) Post(ctx context.Context, path string, data interface{}) (*http.Response, error) {
	return c.DoRequest(ctx, http.MethodPost, normalizePath(path), data)
}

func (c *GitlabNetClient) prepareRequest(request *http.Request) error {
	user, password := c.user, c.password
	if user != "" && password != "" {
		request.SetBasicAuth(user, password)
	}

	claims := jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtTTL)),
	}
	secretBytes := []byte(strings.TrimSpace(c.secret))
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secretBytes)
	if err != nil {
		return err
	}
	request.Header.Set(apiSecretHeaderName, tokenString)

	originalRemoteIP, ok := request.Context().Value(OriginalRemoteIPContextKey{}).(string)
	if ok {
		request.Header.Add("X-Forwarded-For", originalRemoteIP)
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("User-Agent", c.userAgent)
	request.Close = true

	return nil
}

func processResult(request *http.Request, response *http.Response, start time.Time, respErr error) error {
	fields := log.Fields{
		"method":      request.Method,
		"url":         request.URL.String(),
		"duration_ms": time.Since(start) / time.Millisecond,
	}
	logger := log.WithContextFields(request.Context(), fields)

	if respErr != nil {
		logger.WithError(respErr).Error("Internal API unreachable")
		return &ApiError{"Internal API unreachable"}
	}

	if response != nil {
		logger = logger.WithField("status", response.StatusCode)
	}
	if err := parseError(response); err != nil {
		logger.WithError(err).Error("Internal API error")
		return err
	}

	if response.ContentLength >= 0 {
		logger = logger.WithField("content_length_bytes", response.ContentLength)
	}

	logger.Info("Finished HTTP request")

	return nil
}

func (c *GitlabNetClient) AppendPath(path string) string {
	return appendPath(c.httpClient.Host, path)
}

func (c *GitlabNetClient) DoRawRequest(request *http.Request) (*http.Response, error) {
	c.prepareRequest(request)

	start := time.Now()

	response, respErr := c.httpClient.HTTPClient.Do(request)

	if err := processResult(request, response, start, respErr); err != nil {
		return nil, err
	}

	return response, nil
}

func (c *GitlabNetClient) DoRequest(ctx context.Context, method, path string, data interface{}) (*http.Response, error) {
	request, err := newRequest(ctx, method, c.httpClient.Host, path, data)
	if err != nil {
		return nil, err
	}

	retryableRequest, err := newRetryableRequest(ctx, method, c.httpClient.Host, path, data)
	if err != nil {
		return nil, err
	}

	c.prepareRequest(request)
	c.prepareRequest(retryableRequest.Request)

	start := time.Now()

	var response *http.Response
	var respErr error
	if c.httpClient.HTTPClient != nil {
		response, respErr = c.httpClient.HTTPClient.Do(request)
	}
	if os.Getenv("FF_GITLAB_SHELL_RETRYABLE_HTTP") == "1" && c.httpClient.RetryableHTTP != nil {
		response, respErr = c.httpClient.RetryableHTTP.Do(retryableRequest)
	}

	if err := processResult(request, response, start, respErr); err != nil {
		return nil, err
	}

	return response, err
}

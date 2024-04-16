// Package client provides a client for interacting with GitLab API
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hashicorp/go-retryablehttp"
)

const (
	internalAPIPath     = "/api/v4/internal"
	apiSecretHeaderName = "Gitlab-Shell-Api-Request" // #nosec G101
	defaultUserAgent    = "GitLab-Shell"
	jwtTTL              = time.Minute
	jwtIssuer           = "gitlab-shell"
)

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Message string `json:"message"`
}

// GitlabNetClient is a client for interacting with GitLab API
type GitlabNetClient struct {
	httpClient *HttpClient
	user       string
	password   string
	secret     string
	userAgent  string
}

// APIError represents an API error
type APIError struct {
	Msg string
}

// OriginalRemoteIPContextKey is used as the key in a Context to set an X-Forwarded-For header in a request
type OriginalRemoteIPContextKey struct{}

func (e *APIError) Error() string {
	return e.Msg
}

// NewGitlabNetClient creates a new GitlabNetClient instance
func NewGitlabNetClient(
	user,
	password,
	secret string,
	httpClient *HttpClient,
) (*GitlabNetClient, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("unsupported protocol")
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

	if !strings.HasPrefix(path, internalAPIPath) {
		path = internalAPIPath + path
	}
	return path
}

func appendPath(host string, path string) string {
	return strings.TrimSuffix(host, "/") + "/" + strings.TrimPrefix(path, "/")
}

func newRequest(ctx context.Context, method, host, path string, data interface{}) (*retryablehttp.Request, error) {
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

func parseError(resp *http.Response, respErr error) error {
	if resp == nil || respErr != nil {
		return &APIError{"Internal API unreachable"}
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 399 {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	parsedResponse := &ErrorResponse{}

	if err := json.NewDecoder(resp.Body).Decode(parsedResponse); err != nil {
		return &APIError{fmt.Sprintf("Internal API error (%v)", resp.StatusCode)}
	}
	return &APIError{parsedResponse.Message}
}

// Get makes a GET request
func (c *GitlabNetClient) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.DoRequest(ctx, http.MethodGet, normalizePath(path), nil)
}

// Post makes a POST request
func (c *GitlabNetClient) Post(ctx context.Context, path string, data interface{}) (*http.Response, error) {
	return c.DoRequest(ctx, http.MethodPost, normalizePath(path), data)
}

// Do executes a request
func (c *GitlabNetClient) Do(request *http.Request) (*http.Response, error) {
	response, respErr := c.httpClient.RetryableHTTP.HTTPClient.Do(request)
	if err := parseError(response, respErr); err != nil {
		return nil, err
	}

	return response, nil
}

// DoRequest executes a request with the given method, path, and data
func (c *GitlabNetClient) DoRequest(ctx context.Context, method, path string, data interface{}) (*http.Response, error) {
	request, err := newRequest(ctx, method, c.httpClient.Host, path, data)
	if err != nil {
		return nil, err
	}

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
		return nil, err
	}
	request.Header.Set(apiSecretHeaderName, tokenString)

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("User-Agent", c.userAgent)

	response, respErr := c.httpClient.RetryableHTTP.Do(request)
	if err := parseError(response, respErr); err != nil {
		return nil, err
	}

	return response, nil
}

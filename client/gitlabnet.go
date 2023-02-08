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

	"github.com/golang-jwt/jwt/v4"
	"github.com/hashicorp/go-retryablehttp"
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
	if respErr != nil {
		return &ApiError{"Internal API unreachable"}
	}

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

func (c *GitlabNetClient) Do(request *http.Request) (*http.Response, error) {
	response, err := c.httpClient.RetryableHTTP.HTTPClient.Do(request)
	if err := parseError(response, err); err != nil {
		return nil, err
	}

	return response, nil
}

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

	response, err := c.httpClient.RetryableHTTP.Do(request)
	if err := parseError(response, err); err != nil {
		return nil, err
	}

	return response, nil
}

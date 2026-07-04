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
	internalAPIPath        = "/api/v4/internal"
	apiSecretHeaderName    = "Gitlab-Shell-Api-Request" // #nosec G101
	defaultUserAgent       = "GitLab-Shell"
	jwtTTL                 = time.Minute
	jwtIssuer              = "gitlab-shell"
	internalAPIUnreachable = "Internal API unreachable"
)

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Message string `json:"message"`
}

// GitlabNetClient is a client for interacting with GitLab API
type GitlabNetClient struct {
	httpClient *HTTPClient
	user       string
	password   string
	secret     string
	userAgent  string
}

// APIError represents an API error
type APIError struct {
	Msg string

	// StatusCode is the HTTP status returned by the internal API, or 0 when the
	// request never produced a response (e.g. a connection failure).
	StatusCode int

	// System reports whether this is an internal API / transport failure
	// (unreachable, a followed redirect, 400, or 5xx) rather than an expected
	// policy response from the API (e.g. "You are not allowed to push").
	// System errors indicate a gitlab-shell/infrastructure problem and should
	// count toward error SLIs; policy responses are expected and should not.
	System bool
}

// OriginalRemoteIPContextKey is used as the key in a Context to set an X-Forwarded-For header in a request
type OriginalRemoteIPContextKey struct{}

func (e *APIError) Error() string {
	return e.Msg
}

// NewSystemAPIError creates an APIError that represents an internal API or
// transport failure (e.g. unreachable host, followed redirect, 400, or 5xx).
// System errors count toward error SLIs; use a plain APIError for
// expected policy responses (e.g. access denied).
func NewSystemAPIError(msg string, statusCode int) *APIError {
	return &APIError{Msg: msg, StatusCode: statusCode, System: true}
}

// NewGitlabNetClient creates a new GitlabNetClient instance
func NewGitlabNetClient(
	user,
	password,
	secret string,
	httpClient *HTTPClient,
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
		return NewSystemAPIError(internalAPIUnreachable, 0)
	}

	// Redirects are never followed for internal API requests (see
	// NewHTTPClientWithOpts). If one of the redirect status codes that Go's
	// client would otherwise follow comes back, the request was misrouted to a
	// host that wants to redirect us; surface it instead of treating it as
	// success and silently parsing the redirect body. Note 300 Multiple Choices
	// is NOT a redirect here: the internal API uses it for custom actions (e.g.
	// Geo) and its body must be parsed normally.
	if IsFollowedRedirect(resp.StatusCode) {
		defer func() { _ = resp.Body.Close() }()
		return NewSystemAPIError(
			fmt.Sprintf("Internal API returned redirect (%d) to %q", resp.StatusCode, resp.Header.Get("Location")),
			resp.StatusCode,
		)
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 399 {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	parsedResponse := &ErrorResponse{}

	if err := json.NewDecoder(resp.Body).Decode(parsedResponse); err != nil {
		// No structured body to interpret. Classify by status code so this agrees
		// with the transport-layer logging (IsSystemErrorStatus): only followed
		// redirects, 400, and 5xx are treated as system failures. A non-system 4xx
		// with an empty or unparseable body (e.g. a 404 for an unknown SSH key) is
		// still an expected client/policy response and must not count toward the
		// error SLIs.
		return &APIError{
			Msg:        fmt.Sprintf("Internal API error (%v)", resp.StatusCode),
			StatusCode: resp.StatusCode,
			System:     IsSystemErrorStatus(resp.StatusCode),
		}
	}
	// A decoded {"message":…} body is a structured response from the API.
	// Classify via IsSystemErrorStatus so logging and SLI classification agree:
	// followed redirects, 400, and 5xx are system failures; other 4xx (e.g. 403
	// access denied, 404 key not found) are expected policy responses.
	return &APIError{
		Msg:        parsedResponse.Message,
		StatusCode: resp.StatusCode,
		System:     IsSystemErrorStatus(resp.StatusCode),
	}
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
	response, respErr := c.httpClient.RetryableHTTP.HTTPClient.Do(request) // #nosec G704 -- request is constructed by internal callers
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

// WithHost returns a shallow copy of the client that sends requests to the
// specified host instead of the default one. The returned client shares the
// same HTTP transport, TLS settings, and authentication credentials.
// This is used for Cells routing where the Topology Service directs
// requests to a specific cell.
func (c *GitlabNetClient) WithHost(host string) *GitlabNetClient {
	clone := *c
	hostCopy := *c.httpClient
	hostCopy.Host = host
	clone.httpClient = &hostCopy
	return &clone
}

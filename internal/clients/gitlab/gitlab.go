package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
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

type Client struct {
	httpClient *HTTPClient
	user       string
	password   string
	secret     string
	userAgent  string
}

type ClientOpts struct {
	GitlabURL             string
	GitLabRelativeURLRoot string
	CAFile                string
	CAPath                string
	ReadTimeoutSeconds    uint64
	Opts                  []HTTPClientOpt

	User     string
	Password string
	Secret   string
}

func New(opts ClientOpts) (*Client, error) {
	httpClient, err := NewHTTPClient(
		opts.GitlabURL,
		opts.GitLabRelativeURLRoot,
		opts.CAFile,
		opts.CAPath,
		opts.ReadTimeoutSeconds,
		opts.Opts,
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpClient: httpClient,
		user:       opts.User,
		secret:     opts.Secret,
		password:   opts.Password,
		userAgent:  defaultUserAgent,
	}, nil
}

// Do executes a request with the given method, path, and data
func (c *Client) Do(ctx context.Context, method, path string, data any) (*http.Response, error) {
	request, err := newRequest(ctx, method, c.httpClient.Host, normalizePath(path), data)
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

func newRequest(ctx context.Context, method, host, path string, data any) (*retryablehttp.Request, error) {
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

// Package gitlab provides an HTTP client for the GitLab internal API.
// It is the replacement for the client and gitlabnet packages and is being
// introduced incrementally as part of the consolidation described in
// https://gitlab.com/gitlab-org/gitlab-shell/-/issues/834.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/v2/httpclient"
	lablog "gitlab.com/gitlab-org/labkit/v2/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

const (
	internalAPIPath    = "/api/v4/internal"
	defaultReadTimeout = 300 * time.Second
)

// Config holds the configuration for the GitLab internal API client.
type Config struct {
	// GitlabURL is the base URL of the GitLab instance.
	GitlabURL string
	// RelativeURLRoot is an optional relative URL root prefix (for Unix socket URLs).
	RelativeURLRoot string
	// User is the HTTP basic auth username.
	User string
	// Password is the HTTP basic auth password.
	Password string
	// Secret is the HS256 JWT signing secret. Must not be empty.
	Secret string
	// CaFile is the path to a custom CA certificate file.
	CaFile string
	// CaPath is the path to a directory of custom CA certificate files.
	CaPath string
	// ReadTimeoutSeconds is the HTTP read timeout. Defaults to 300s when zero.
	ReadTimeoutSeconds uint64
}

// Client is an HTTP client for the GitLab internal API.
type Client struct {
	inner    *httpclient.Client
	host     string
	user     string
	password string
	secret   string
}

// New creates a new Client from the given Config.
func New(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config must not be nil")
	}

	if strings.TrimSpace(cfg.Secret) == "" {
		return nil, errors.New("secret must not be empty")
	}

	transport, host, err := buildTransport(cfg)
	if err != nil {
		return nil, err
	}

	// Layer cross-cutting concerns on top of the base transport, innermost first:
	//   1. forwardedIPTransport — propagates X-Forwarded-For from context so that
	//      GitLab can log the original client IP for SSH-over-HTTP connections.
	//   2. metrics.NewRoundTripper — instruments every request with Prometheus
	//      counters/histograms, matching the old config.HTTPClient() behavior.
	//   3. correlation.NewInstrumentedRoundTripper — injects the correlation ID
	//      from context as the X-Request-Id header, matching the old transport chain.
	//
	// Retries are handled at the call site via httpclient.Client.DoWithRetry.
	transport = &forwardedIPTransport{next: transport}
	transport = metrics.NewRoundTripper(transport)
	transport = correlation.NewInstrumentedRoundTripper(transport)

	timeout := time.Duration(cfg.ReadTimeoutSeconds) * time.Second // #nosec G115
	if cfg.ReadTimeoutSeconds == 0 {
		timeout = defaultReadTimeout
	}

	inner := httpclient.NewWithConfig(&httpclient.Config{
		Transport: transport,
		Timeout:   timeout,
	})

	return &Client{
		inner:    inner,
		host:     host,
		user:     cfg.User,
		password: cfg.Password,
		secret:   cfg.Secret,
	}, nil
}

// Get makes a GET request to the given internal API path.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

// Post makes a POST request to the given internal API path with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

// do is the single request path for all outbound calls. It exists to keep
// Get/Post thin and ensure that header injection (JWT, basic auth, User-Agent)
// is applied consistently regardless of the HTTP method. During the migration
// from client.GitlabNetClient, additional methods (PUT, DELETE, …) can be
// added here without duplicating the auth logic.
func (c *Client) do(ctx context.Context, method, apiPath string, data any) (*http.Response, error) {
	normalized, err := normalizePath(apiPath)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(c.host, "/") + normalized

	var (
		bodyReader io.Reader
		encoded    []byte
	)
	if data != nil {
		var marshalErr error
		encoded, marshalErr = json.Marshal(data)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshaling request body: %w", marshalErr)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	// Provide GetBody so DoWithRetry can restore the request body between
	// retry attempts. bytes.NewReader supports Reset but http.Request.Body is
	// an io.ReadCloser consumed after the first attempt; GetBody is the
	// standard mechanism for re-creating it.
	if len(encoded) > 0 {
		snapshot := encoded
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(snapshot)), nil
		}
	}

	if err = c.setHeaders(req); err != nil {
		return nil, err
	}

	resp, err := c.inner.DoWithRetry(req, nil)
	if err != nil {
		slog.ErrorContext(ctx, "Internal API unreachable", lablog.ErrorMessage(err.Error()))
		return nil, &client.APIError{Msg: "Internal API unreachable"}
	}
	return resp, nil
}

// setHeaders stamps every outbound request with the three auth/identity
// signals that the GitLab internal API requires:
//
//   - Basic auth — used by some GitLab deployments that sit behind an
//     nginx auth_basic gate; mirrored from HTTPSettingsConfig.User/Password
//     in the existing config package. Both username and password must be
//     non-empty, matching the behavior of the old client.GitlabNetClient.
//   - JWT bearer token — the primary machine-to-machine secret; the Rails
//     side validates the HS256 signature and rejects requests whose token has
//     expired or was signed with the wrong key.
//   - User-Agent — helps GitLab ops distinguish gitlab-shell traffic in
//     access logs; kept identical to the value used by client.GitlabNetClient
//     so log-based dashboards do not need updating during the migration.
func (c *Client) setHeaders(req *http.Request) error {
	if c.user != "" && c.password != "" {
		req.SetBasicAuth(c.user, c.password)
	}

	token, err := c.jwtToken()
	if err != nil {
		return err
	}

	req.Header.Set(apiSecretHeaderName, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	return nil
}

// normalizePath ensures every path is rooted under /api/v4/internal. This
// mirrors the logic in client.GitlabNetClient so that callers migrated to
// this package can pass the same short paths (e.g. "/check", "lfs/objects")
// without any changes at the call site.
//
// path.Clean is applied after prefixing to collapse any traversal segments
// (e.g. "/../") and repeated slashes. An error is returned if the cleaned
// result no longer starts with /api/v4/internal, which would indicate a
// traversal attempt escaping the internal API prefix.
func normalizePath(p string) (string, error) {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasPrefix(p, internalAPIPath) {
		p = internalAPIPath + p
	}
	cleaned := path.Clean(p)
	if !strings.HasPrefix(cleaned, internalAPIPath) {
		return "", fmt.Errorf("path %q escapes the internal API prefix", p)
	}
	return cleaned, nil
}

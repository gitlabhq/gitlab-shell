package gitlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
)

const testSecret = "test-secret-value"

func newTestClient(t *testing.T, srv *httptest.Server) *gitlab.Client {
	t.Helper()
	c, err := gitlab.New(&gitlab.Config{
		GitlabURL:          srv.URL,
		Secret:             testSecret,
		ReadTimeoutSeconds: 10,
	})
	require.NoError(t, err)
	return c
}

func TestNew_NilConfig(t *testing.T) {
	_, err := gitlab.New(nil)
	require.ErrorContains(t, err, "config must not be nil")
}

func TestNew_EmptySecret(t *testing.T) {
	_, err := gitlab.New(&gitlab.Config{GitlabURL: "http://localhost", Secret: ""})
	require.ErrorContains(t, err, "secret must not be empty")
}

func TestNew_WhitespaceOnlySecret(t *testing.T) {
	_, err := gitlab.New(&gitlab.Config{GitlabURL: "http://localhost", Secret: "   \n"})
	require.ErrorContains(t, err, "secret must not be empty")
}

func TestNew_UnknownURLPrefix(t *testing.T) {
	_, err := gitlab.New(&gitlab.Config{GitlabURL: "ftp://example.com", Secret: testSecret})
	require.ErrorContains(t, err, "unknown GitLab URL prefix")
}

func TestGet_SetsRequiredHeaders(t *testing.T) {
	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	resp, err := c.Get(context.Background(), "/check")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", capturedReq.Header.Get("Content-Type"))
	require.Equal(t, "GitLab-Shell", capturedReq.Header.Get("User-Agent"))
	require.Equal(t, "/api/v4/internal/check", capturedReq.URL.Path)

	// JWT header must be present and valid
	tokenStr := capturedReq.Header.Get("Gitlab-Shell-Api-Request")
	require.NotEmpty(t, tokenStr)

	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(_ *jwt.Token) (any, error) {
		return []byte(testSecret), nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	require.True(t, ok)
	require.Equal(t, "gitlab-shell", claims.Issuer)
	require.WithinDuration(t, time.Now(), claims.IssuedAt.Time, 5*time.Second)
}

func TestPost_SendsJSONBody(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	resp, err := c.Post(context.Background(), "/lfs/objects", map[string]string{"key": "value"})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, map[string]string{"key": "value"}, received)
}

func TestGet_NormalizesPath(t *testing.T) {
	paths := []struct {
		input    string
		wantPath string
	}{
		{"/check", "/api/v4/internal/check"},
		{"check", "/api/v4/internal/check"},
		{"/api/v4/internal/check", "/api/v4/internal/check"},
		// Traversal segments within the prefix are collapsed safely.
		{"/check/../other", "/api/v4/internal/other"},
	}

	for _, tc := range paths {
		t.Run(tc.input, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			c := newTestClient(t, srv)
			resp, err := c.Get(context.Background(), tc.input)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tc.wantPath, gotPath)
		})
	}
}

func TestGet_PathTraversalRejected(t *testing.T) {
	// Paths that escape /api/v4/internal after cleaning must be rejected
	// rather than silently rewritten.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	resp, err := c.Get(context.Background(), "/../../../etc/passwd")
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	require.ErrorContains(t, err, "escapes the internal API prefix")
}

func TestNew_BasicAuth(t *testing.T) {
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := gitlab.New(&gitlab.Config{
		GitlabURL: srv.URL,
		User:      "alice",
		Password:  "hunter2",
		Secret:    testSecret,
	})
	require.NoError(t, err)

	resp, err := c.Get(context.Background(), "/check")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, "alice", gotUser)
	require.Equal(t, "hunter2", gotPass)
}

func TestNew_BasicAuthEmptyPassword(t *testing.T) {
	// Basic auth requires both a username and a password, matching the old
	// client.GitlabNetClient behavior. A username alone is not sufficient.
	var gotAuthHeader bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, gotAuthHeader = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := gitlab.New(&gitlab.Config{
		GitlabURL: srv.URL,
		User:      "alice",
		Password:  "",
		Secret:    testSecret,
	})
	require.NoError(t, err)

	resp, err := c.Get(context.Background(), "/check")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.False(t, gotAuthHeader)
}

func TestNew_UnixSocket(t *testing.T) {
	// Ensure a unix:// URL is accepted and produces the correct base host.
	// We don't spin up a real socket; just verify New() doesn't error.
	_, err := gitlab.New(&gitlab.Config{
		GitlabURL: "http+unix:///tmp/gitlab.sock",
		Secret:    testSecret,
	})
	require.NoError(t, err)
}

func TestNew_DefaultTimeout(t *testing.T) {
	// ReadTimeoutSeconds == 0 should not error; default 300s is applied internally.
	_, err := gitlab.New(&gitlab.Config{
		GitlabURL: "http://localhost",
		Secret:    testSecret,
	})
	require.NoError(t, err)
}

func TestPost_WithSecretWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Secrets with surrounding whitespace should be trimmed before signing.
	c, err := gitlab.New(&gitlab.Config{
		GitlabURL: srv.URL,
		Secret:    "  " + testSecret + "\n",
	})
	require.NoError(t, err)

	resp, err := c.Post(context.Background(), "/check", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNew_HTTPS_MissingCaFile(t *testing.T) {
	_, err := gitlab.New(&gitlab.Config{
		GitlabURL: "https://localhost",
		Secret:    testSecret,
		CaFile:    "/nonexistent/ca.pem",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "reading CA file")
}

func TestGet_PathAlreadyPrefixed(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	resp, err := c.Get(context.Background(), "/api/v4/internal/check")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Path must not be double-prefixed.
	require.False(t, strings.HasPrefix(gotPath, "/api/v4/internal/api/v4/internal"),
		"path was double-prefixed: %s", gotPath)
	require.Equal(t, "/api/v4/internal/check", gotPath)
}

func TestGet_ForwardsRemoteIP(t *testing.T) {
	var gotForwardedFor string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	ctx := context.WithValue(context.Background(), client.OriginalRemoteIPContextKey{}, "1.2.3.4")
	resp, err := c.Get(ctx, "/check")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, "1.2.3.4", gotForwardedFor)
}

func TestGet_RetriesOn503(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := gitlab.New(&gitlab.Config{
		GitlabURL:          srv.URL,
		Secret:             testSecret,
		ReadTimeoutSeconds: 10,
	})
	require.NoError(t, err)

	resp, err := c.Get(context.Background(), "/check")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 3, attempts) // 2 failures + 1 success
}

func TestGet_NoForwardedIPWithoutContext(t *testing.T) {
	var gotForwardedFor string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)

	resp, err := c.Get(context.Background(), "/check")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Empty(t, gotForwardedFor)
}

func TestGet_NetworkErrorReturnsAPIError(t *testing.T) {
	// Close the server before making the request to force a connection-refused error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := srv.URL
	srv.Close()

	c, err := gitlab.New(&gitlab.Config{
		GitlabURL: url,
		Secret:    testSecret,
	})
	require.NoError(t, err)

	resp, err := c.Get(context.Background(), "/check")
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)

	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "Internal API unreachable", apiErr.Msg)
}

func TestWithHost(t *testing.T) {
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"source":"original"}`))
	}))
	defer originalServer.Close()

	newHostServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"source":"new-host"}`))
	}))
	defer newHostServer.Close()

	c := newTestClient(t, originalServer)

	t.Run("clone sends requests to new host", func(t *testing.T) {
		clone := c.WithHost(newHostServer.URL)

		resp, err := clone.Get(context.Background(), "/check")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var dst struct {
			Source string `json:"source"`
		}
		require.NoError(t, gitlab.ParseJSON(resp, &dst))
		require.Equal(t, "new-host", dst.Source)
	})

	t.Run("original client is unaffected", func(t *testing.T) {
		_ = c.WithHost(newHostServer.URL)

		resp, err := c.Get(context.Background(), "/check")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var dst struct {
			Source string `json:"source"`
		}
		require.NoError(t, gitlab.ParseJSON(resp, &dst))
		require.Equal(t, "original", dst.Source)
	})
}

func TestParseJSON_SuccessDecodes(t *testing.T) {
	body := bytes.NewBufferString(`{"key":"value"}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(body),
	}

	var dst struct {
		Key string `json:"key"`
	}
	err := gitlab.ParseJSON(resp, &dst)
	require.NoError(t, err)
	require.Equal(t, "value", dst.Key)
}

func TestParseJSON_BadJSONReturnsParsingFailed(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("not json")),
	}
	err := gitlab.ParseJSON(resp, &struct{}{})
	require.EqualError(t, err, "parsing failed")
}

func TestParseJSON_4xxWithMessageReturnsAPIError(t *testing.T) {
	body := bytes.NewBufferString(`{"message":"Not allowed!"}`)
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       io.NopCloser(body),
	}
	err := gitlab.ParseJSON(resp, &struct{}{})

	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "Not allowed!", apiErr.Msg)
}

func TestParseJSON_5xxWithoutMessageReturnsAPIError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString("")),
	}
	err := gitlab.ParseJSON(resp, &struct{}{})

	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "Internal API error (500)", apiErr.Msg)
}

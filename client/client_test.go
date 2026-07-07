package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

var (
	secret          = "sssh, it's a secret"
	defaultHTTPOpts = []HTTPClientOpt{WithHTTPRetryOpts(time.Millisecond, time.Millisecond, 2)}
)

func TestClients(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	testCases := []struct {
		desc            string
		relativeURLRoot string
		caFile          string
		server          func(*testing.T, []testserver.TestRequestHandler) string
		secret          string
	}{
		{
			desc:   "Socket client",
			server: testserver.StartSocketHTTPServer,
			secret: secret,
		},
		{
			desc:            "Socket client with a relative URL at /",
			relativeURLRoot: "/",
			server:          testserver.StartSocketHTTPServer,
			secret:          secret,
		},
		{
			desc:            "Socket client with relative URL at /gitlab",
			relativeURLRoot: "/gitlab",
			server:          testserver.StartSocketHTTPServer,
			secret:          secret,
		},
		{
			desc:   "Http client",
			server: testserver.StartHTTPServer,
			secret: secret,
		},
		{
			desc:   "Https client",
			caFile: path.Join(testRoot, "certs/valid/server.crt"),
			server: func(t *testing.T, handlers []testserver.TestRequestHandler) string {
				return testserver.StartHTTPSServer(t, handlers, "")
			},
			secret: secret,
		},
		{
			desc:   "Secret with newlines",
			caFile: path.Join(testRoot, "certs/valid/server.crt"),
			server: func(t *testing.T, handlers []testserver.TestRequestHandler) string {
				return testserver.StartHTTPSServer(t, handlers, "")
			},
			secret: "\n" + secret + "\n",
		},
		{
			desc:   "Retry client",
			server: testserver.StartRetryHTTPServer,
			secret: secret,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			url := tc.server(t, buildRequests(t, tc.relativeURLRoot))

			httpClient, err := NewHTTPClientWithOpts(url, tc.relativeURLRoot, tc.caFile, "", 1, defaultHTTPOpts)
			require.NoError(t, err)

			client, err := NewGitlabNetClient("", "", tc.secret, httpClient)
			require.NoError(t, err)

			testBrokenRequest(t, client)
			testSuccessfulGet(t, client)
			testSuccessfulPost(t, client)
			testMissing(t, client)
			testErrorMessage(t, client)
			testJWTAuthenticationHeader(t, client)
			testXForwardedForHeader(t, client)
			testHostWithTrailingSlash(t, client)
		})
	}
}

func testSuccessfulGet(t *testing.T, client *GitlabNetClient) {
	t.Run("Successful get", func(t *testing.T) {
		response, err := client.Get(context.Background(), "/hello")
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "Hello", string(responseBody))
	})
}

func testSuccessfulPost(t *testing.T, client *GitlabNetClient) {
	t.Run("Successful Post", func(t *testing.T) {
		data := map[string]string{"key": "value"}

		response, err := client.Post(context.Background(), "/post_endpoint", data)
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "Echo: {\"key\":\"value\"}", string(responseBody))
	})
}

func testMissing(t *testing.T, client *GitlabNetClient) {
	t.Run("Missing error for GET", func(t *testing.T) {
		response, err := client.Get(context.Background(), "/missing")
		require.EqualError(t, err, "Internal API error (404)")
		if response != nil {
			response.Body.Close()
		}
		require.Nil(t, response)
	})

	t.Run("Missing error for POST", func(t *testing.T) {
		response, err := client.Post(context.Background(), "/missing", map[string]string{})
		require.EqualError(t, err, "Internal API error (404)")
		if response != nil {
			response.Body.Close()
		}
		require.Nil(t, response)
	})
}

func testErrorMessage(t *testing.T, client *GitlabNetClient) {
	t.Run("Error with message for GET", func(t *testing.T) {
		response, err := client.Get(context.Background(), "/error")
		require.EqualError(t, err, "Don't do that")
		if response != nil {
			response.Body.Close()
		}
		require.Nil(t, response)
	})

	t.Run("Error with message for POST", func(t *testing.T) {
		response, err := client.Post(context.Background(), "/error", map[string]string{})
		require.EqualError(t, err, "Don't do that")
		if response != nil {
			response.Body.Close()
		}
		require.Nil(t, response)
	})
}

func testBrokenRequest(t *testing.T, client *GitlabNetClient) {
	t.Run("Broken request for GET", func(t *testing.T) {
		response, err := client.Get(context.Background(), "/broken")
		require.EqualError(t, err, "Internal API unreachable")
		if response != nil {
			response.Body.Close()
		}
		require.Nil(t, response)
	})

	t.Run("Broken request for POST", func(t *testing.T) {
		response, err := client.Post(context.Background(), "/broken", map[string]string{})
		require.EqualError(t, err, "Internal API unreachable")
		if response != nil {
			response.Body.Close()
		}
		require.Nil(t, response)
	})
}

func testJWTAuthenticationHeader(t *testing.T, client *GitlabNetClient) {
	verifyJWTToken := func(t *testing.T, response *http.Response) {
		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		claims := &jwt.RegisteredClaims{}
		token, err := jwt.ParseWithClaims(string(responseBody), claims, func(_ *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		require.NoError(t, err)
		require.True(t, token.Valid)
		require.Equal(t, "gitlab-shell", claims.Issuer)
		require.WithinDuration(t, time.Now().Truncate(time.Second), claims.IssuedAt.Time, time.Second)
		require.WithinDuration(t, time.Now().Truncate(time.Second).Add(time.Minute), claims.ExpiresAt.Time, time.Second)
	}

	t.Run("JWT authentication headers for GET", func(t *testing.T) {
		response, err := client.Get(context.Background(), "/jwt_auth")
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		verifyJWTToken(t, response)
	})

	t.Run("JWT authentication headers for POST", func(t *testing.T) {
		response, err := client.Post(context.Background(), "/jwt_auth", map[string]string{})
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		verifyJWTToken(t, response)
	})
}

func testXForwardedForHeader(t *testing.T, client *GitlabNetClient) {
	t.Run("X-Forwarded-For Header inserted if original address in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), OriginalRemoteIPContextKey{}, "196.7.0.238")
		response, err := client.Get(ctx, "/x_forwarded_for")
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "196.7.0.238", string(responseBody))
	})
}

func testHostWithTrailingSlash(t *testing.T, client *GitlabNetClient) {
	oldHost := client.httpClient.Host
	client.httpClient.Host = oldHost + "/"

	testSuccessfulGet(t, client)
	testSuccessfulPost(t, client)

	client.httpClient.Host = oldHost
}

func buildRequests(t *testing.T, relativeURLRoot string) []testserver.TestRequestHandler {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/hello",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "Hello")
			},
		},
		{
			Path: "/api/v4/internal/post_endpoint",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)

				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()

				assert.NoError(t, err)

				fmt.Fprint(w, "Echo: "+string(b))
			},
		},
		{
			Path: "/api/v4/internal/jwt_auth",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, r.Header.Get(apiSecretHeaderName))
			},
		},
		{
			Path: "/api/v4/internal/x_forwarded_for",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, r.Header.Get("X-Forwarded-For"))
			},
		},
		{
			Path: "/api/v4/internal/error",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				body := map[string]string{
					"message": "Don't do that",
				}
				json.NewEncoder(w).Encode(body)
			},
		},
		{
			Path: "/api/v4/internal/broken",
			Handler: func(_ http.ResponseWriter, _ *http.Request) {
				panic("Broken")
			},
		},
	}

	relativeURLRoot = strings.Trim(relativeURLRoot, "/")
	if relativeURLRoot != "" {
		for i, r := range requests {
			requests[i].Path = fmt.Sprintf("/%s%s", relativeURLRoot, r.Path)
		}
	}

	return requests
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	// Mimics the Cells incident: the configured host issues a 301 (e.g. a public
	// URL bouncing the internal API path). The client must NOT follow it, since
	// following a 301 downgrades the POST to a GET and silently misroutes it.
	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		fmt.Fprint(w, "should never be reached")
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	httpClient, err := NewHTTPClientWithOpts(redirector.URL, "", "", "", 1, defaultHTTPOpts)
	require.NoError(t, err)

	client, err := NewGitlabNetClient("", "", secret, httpClient)
	require.NoError(t, err)

	t.Run("POST does not follow redirect and surfaces an error", func(t *testing.T) {
		resp, err := client.Post(context.Background(), "/allowed", map[string]string{})
		if resp != nil {
			resp.Body.Close()
		}
		require.Nil(t, resp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "301")
		require.Zero(t, atomic.LoadInt32(&targetHits), "redirect target must not be reached")
	})
}

func TestParseErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		desc       string
		status     int   // 0 simulates a connection failure (nil response, non-nil error)
		respErr    error // request error to use when status == 0; defaults to a connection-refused error
		body       string
		wantSystem bool
		wantCode   int
	}{
		{
			desc:       "connection failure is a system error",
			status:     0,
			wantSystem: true,
			wantCode:   0,
		},
		{
			desc:       "canceled context is a client-side error",
			status:     0,
			respErr:    context.Canceled,
			wantSystem: false,
			wantCode:   0,
		},
		{
			desc:       "wrapped canceled context is a client-side error",
			status:     0,
			respErr:    fmt.Errorf("Get %q: %w", "http://example.com", context.Canceled),
			wantSystem: false,
			wantCode:   0,
		},
		{
			desc:       "deadline exceeded is a system error",
			status:     0,
			respErr:    context.DeadlineExceeded,
			wantSystem: true,
			wantCode:   0,
		},
		{
			desc:       "4xx with a structured message is a policy response",
			status:     http.StatusForbidden,
			body:       `{"message":"You are not allowed to push"}`,
			wantSystem: false,
			wantCode:   http.StatusForbidden,
		},
		{
			desc:       "5xx with a structured message is a system error",
			status:     http.StatusInternalServerError,
			body:       `{"message":"boom"}`,
			wantSystem: true,
			wantCode:   http.StatusInternalServerError,
		},
		{
			desc:       "4xx with a non-JSON body is a policy response",
			status:     http.StatusNotFound,
			body:       "<html>not found</html>",
			wantSystem: false,
			wantCode:   http.StatusNotFound,
		},
		{
			desc:       "5xx with a non-JSON body is a system error",
			status:     http.StatusBadGateway,
			body:       "<html>bad gateway</html>",
			wantSystem: true,
			wantCode:   http.StatusBadGateway,
		},
		{
			desc:       "400 with a JSON body is a system error",
			status:     http.StatusBadRequest,
			body:       `{"message":"bad request"}`,
			wantSystem: true,
			wantCode:   http.StatusBadRequest,
		},
		{
			desc:       "followed redirect is a system error",
			status:     http.StatusMovedPermanently,
			wantSystem: true,
			wantCode:   http.StatusMovedPermanently,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			var resp *http.Response
			var respErr error
			if tc.status == 0 {
				respErr = tc.respErr
				if respErr == nil {
					respErr = errors.New("dial tcp: connection refused")
				}
			} else {
				resp = &http.Response{
					StatusCode: tc.status,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}
			}

			err := parseError(resp, respErr)

			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, tc.wantSystem, apiErr.System)
			require.Equal(t, tc.wantCode, apiErr.StatusCode)
		})
	}
}

func TestWithHost(t *testing.T) {
	// Set up two test servers: one for the original host, one for the new host.
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "original")
	}))
	defer originalServer.Close()

	newHostServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "new-host")
	}))
	defer newHostServer.Close()

	httpClient, err := NewHTTPClientWithOpts(originalServer.URL, "", "", "", 1, defaultHTTPOpts)
	require.NoError(t, err)

	client, err := NewGitlabNetClient("", "", secret, httpClient)
	require.NoError(t, err)

	t.Run("clone sends requests to new host", func(t *testing.T) {
		clone := client.WithHost(newHostServer.URL)

		resp, err := clone.Get(context.Background(), "/hello")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "new-host", string(body))
	})

	t.Run("original client is unaffected", func(t *testing.T) {
		_ = client.WithHost(newHostServer.URL)

		resp, err := client.Get(context.Background(), "/hello")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "original", string(body))
	})
}

func TestSignShellJWT(t *testing.T) {
	t.Run("generates valid JWT with gl_id claim", func(t *testing.T) {
		tokenString, err := SignShellJWT(secret, "user-1")
		require.NoError(t, err)
		require.NotEmpty(t, tokenString)

		claims := &ShellClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		require.NoError(t, err)
		require.True(t, token.Valid)
		require.Equal(t, "gitlab-shell", claims.Issuer)
		require.Equal(t, "user-1", claims.GlID)
		require.WithinDuration(t, time.Now().Truncate(time.Second), claims.IssuedAt.Time, time.Second)
		require.WithinDuration(t, time.Now().Truncate(time.Second).Add(time.Minute), claims.ExpiresAt.Time, time.Second)
	})

	t.Run("trims whitespace from secret", func(t *testing.T) {
		tokenString, err := SignShellJWT("\n"+secret+"\n", "key-1")
		require.NoError(t, err)

		claims := &ShellClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		require.NoError(t, err)
		require.True(t, token.Valid)
	})
}

func TestRetryOnFailure(t *testing.T) {
	reqAttempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reqAttempts++
		w.WriteHeader(500)
	}))
	defer srv.Close()

	httpClient, err := NewHTTPClientWithOpts(srv.URL, "/", "", "", 1, defaultHTTPOpts)
	require.NoError(t, err)
	require.NotNil(t, httpClient.RetryableHTTP)
	client, err := NewGitlabNetClient("", "", "", httpClient)
	require.NoError(t, err)

	resp, err := client.Get(context.Background(), "/")
	if resp != nil {
		resp.Body.Close()
	}
	require.EqualError(t, err, "Internal API unreachable")
	require.Equal(t, 3, reqAttempts)
}

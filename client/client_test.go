package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
		{"Socket client", "", "", testserver.StartSocketHttpServer, secret},
		{"Socket client with a relative URL at /", "/", "", testserver.StartSocketHttpServer, secret},
		{"Socket client with relative URL at /gitlab", "/gitlab", "", testserver.StartSocketHttpServer, secret},
		{"Http client", "", "", testserver.StartHttpServer, secret},
		{"Https client", "", path.Join(testRoot, "certs/valid/server.crt"), startHttpsServer, secret},
		{"Secret with newlines", "", path.Join(testRoot, "certs/valid/server.crt"), startHttpsServer, "\n" + secret + "\n"},
		{"Retry client", "", "", testserver.StartRetryHttpServer, secret},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			url := tc.server(t, buildRequests(t, tc.relativeURLRoot))

			httpClient, err := NewHTTPClientWithOpts(url, tc.relativeURLRoot, tc.caFile, "", 1, defaultHTTPOpts)
			require.NoError(t, err)

			client, err := NewGitlabNetClient("", "", tc.secret, httpClient)
			require.NoError(t, err)

			runClientTests(t, client)
		})
	}
}

func startHttpsServer(t *testing.T, handlers []testserver.TestRequestHandler) string {
	return testserver.StartHttpsServer(t, handlers, "")
}

func runClientTests(t *testing.T, client *GitlabNetClient) {
	t.Run("Test successful GET", func(t *testing.T) { testSuccessfulGet(t, client) })
	t.Run("Test successful POST", func(t *testing.T) { testSuccessfulPost(t, client) })
	t.Run("Test missing endpoints", func(t *testing.T) { testMissing(t, client) })
	t.Run("Test error messages", func(t *testing.T) { testErrorMessage(t, client) })
	t.Run("Test broken requests", func(t *testing.T) { testBrokenRequest(t, client) })
	t.Run("Test JWT authentication header", func(t *testing.T) { testJWTAuthenticationHeader(t, client) })
	t.Run("Test X-Forwarded-For header", func(t *testing.T) { testXForwardedForHeader(t, client) })
	t.Run("Test host with trailing slash", func(t *testing.T) { testHostWithTrailingSlash(t, client) })
}

func testSuccessfulGet(t *testing.T, client *GitlabNetClient) {
	response, err := client.Get(context.Background(), "/hello")
	require.NoError(t, err)
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "Hello", string(responseBody))
}

func testSuccessfulPost(t *testing.T, client *GitlabNetClient) {
	data := map[string]string{"key": "value"}
	response, err := client.Post(context.Background(), "/post_endpoint", data)
	require.NoError(t, err)
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "Echo: {\"key\":\"value\"}", string(responseBody))
}

func testMissing(t *testing.T, client *GitlabNetClient) {
	testEndpointError(t, client, "/missing", "Internal API error (404)")
}

func testErrorMessage(t *testing.T, client *GitlabNetClient) {
	testEndpointError(t, client, "/error", "Don't do that")
}

func testBrokenRequest(t *testing.T, client *GitlabNetClient) {
	testEndpointError(t, client, "/broken", "Internal API unreachable")
}

func testEndpointError(t *testing.T, client *GitlabNetClient, endpoint string, expectedError string) {
	t.Run("GET "+endpoint, func(t *testing.T) {
		response, err := client.Get(context.Background(), endpoint)
		require.EqualError(t, err, expectedError)
		require.Nil(t, response)
	})

	t.Run("POST "+endpoint, func(t *testing.T) {
		response, err := client.Post(context.Background(), endpoint, map[string]string{})
		require.EqualError(t, err, expectedError)
		require.Nil(t, response)
	})
}

func testJWTAuthenticationHeader(t *testing.T, client *GitlabNetClient) {
	t.Run("GET JWT authentication headers", func(t *testing.T) {
		response, err := client.Get(context.Background(), "/jwt_auth")
		require.NoError(t, err)
		defer response.Body.Close()
		verifyJWTToken(t, response)
	})

	t.Run("POST JWT authentication headers", func(t *testing.T) {
		response, err := client.Post(context.Background(), "/jwt_auth", map[string]string{})
		require.NoError(t, err)
		defer response.Body.Close()
		verifyJWTToken(t, response)
	})
}

func verifyJWTToken(t *testing.T, response *http.Response) {
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(string(responseBody), claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)
	require.Equal(t, "gitlab-shell", claims.Issuer)
	require.WithinDuration(t, time.Now().Truncate(time.Second), claims.IssuedAt.Time, time.Second)
	require.WithinDuration(t, time.Now().Truncate(time.Second).Add(time.Minute), claims.ExpiresAt.Time, time.Second)
}

func testXForwardedForHeader(t *testing.T, client *GitlabNetClient) {
	t.Run("X-Forwarded-For Header inserted if original address in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), OriginalRemoteIPContextKey{}, "196.7.0.238")
		response, err := client.Get(ctx, "/x_forwarded_for")
		require.NoError(t, err)
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
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				fmt.Fprint(w, "Hello")
			},
		},
		{
			Path: "/api/v4/internal/post_endpoint",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				b, err := io.ReadAll(r.Body)
				defer r.Body.Close()
				require.NoError(t, err)
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
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				body := map[string]string{"message": "Don't do that"}

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

func TestClients(t *testing.T) {
	testDirCleanup, err := testhelper.PrepareTestRootDir()
	require.NoError(t, err)
	defer testDirCleanup()

	testCases := []struct {
		desc            string
		relativeURLRoot string
		caFile          string
		server          func(*testing.T, []testserver.TestRequestHandler) string
	}{
		{
			desc:   "Socket client",
			server: testserver.StartSocketHttpServer,
		},
		{
			desc:            "Socket client with a relative URL at /",
			relativeURLRoot: "/",
			server:          testserver.StartSocketHttpServer,
		},
		{
			desc:            "Socket client with relative URL at /gitlab",
			relativeURLRoot: "/gitlab",
			server:          testserver.StartSocketHttpServer,
		},
		{
			desc:   "Http client",
			server: testserver.StartHttpServer,
		},
		{
			desc:   "Https client",
			caFile: path.Join(testhelper.TestRoot, "certs/valid/server.crt"),
			server: func(t *testing.T, handlers []testserver.TestRequestHandler) string {
				return testserver.StartHttpsServer(t, handlers, "")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			url := tc.server(t, buildRequests(t, tc.relativeURLRoot))

			secret := "sssh, it's a secret"

			httpClient := NewHTTPClient(url, tc.relativeURLRoot, tc.caFile, "", false, 1)

			client, err := NewGitlabNetClient("", "", secret, httpClient)
			require.NoError(t, err)

			testBrokenRequest(t, client)
			testSuccessfulGet(t, client)
			testSuccessfulPost(t, client)
			testMissing(t, client)
			testErrorMessage(t, client)
			testAuthenticationHeader(t, client)
		})
	}
}

func testSuccessfulGet(t *testing.T, client *GitlabNetClient) {
	t.Run("Successful get", func(t *testing.T) {
		hook := testhelper.SetupLogger()
		response, err := client.Get(context.Background(), "/hello")
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, string(responseBody), "Hello")

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		require.Equal(t, 1, len(entries))
		require.Equal(t, logrus.InfoLevel, entries[0].Level)
		require.Contains(t, entries[0].Message, "method=GET")
		require.Contains(t, entries[0].Message, "status=200")
		require.Contains(t, entries[0].Message, "content_length_bytes=")
		require.Contains(t, entries[0].Message, "Finished HTTP request")
		require.Contains(t, entries[0].Message, "correlation_id=")
	})
}

func testSuccessfulPost(t *testing.T, client *GitlabNetClient) {
	t.Run("Successful Post", func(t *testing.T) {
		hook := testhelper.SetupLogger()
		data := map[string]string{"key": "value"}

		response, err := client.Post(context.Background(), "/post_endpoint", data)
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "Echo: {\"key\":\"value\"}", string(responseBody))

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		require.Equal(t, 1, len(entries))
		require.Equal(t, logrus.InfoLevel, entries[0].Level)
		require.Contains(t, entries[0].Message, "method=POST")
		require.Contains(t, entries[0].Message, "status=200")
		require.Contains(t, entries[0].Message, "content_length_bytes=")
		require.Contains(t, entries[0].Message, "Finished HTTP request")
		require.Contains(t, entries[0].Message, "correlation_id=")
	})
}

func testMissing(t *testing.T, client *GitlabNetClient) {
	t.Run("Missing error for GET", func(t *testing.T) {
		hook := testhelper.SetupLogger()
		response, err := client.Get(context.Background(), "/missing")
		require.EqualError(t, err, "Internal API error (404)")
		require.Nil(t, response)

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		require.Equal(t, 1, len(entries))
		require.Equal(t, logrus.InfoLevel, entries[0].Level)
		require.Contains(t, entries[0].Message, "method=GET")
		require.Contains(t, entries[0].Message, "status=404")
		require.Contains(t, entries[0].Message, "Internal API error")
		require.Contains(t, entries[0].Message, "correlation_id=")
	})

	t.Run("Missing error for POST", func(t *testing.T) {
		hook := testhelper.SetupLogger()
		response, err := client.Post(context.Background(), "/missing", map[string]string{})
		require.EqualError(t, err, "Internal API error (404)")
		require.Nil(t, response)

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		require.Equal(t, 1, len(entries))
		require.Equal(t, logrus.InfoLevel, entries[0].Level)
		require.Contains(t, entries[0].Message, "method=POST")
		require.Contains(t, entries[0].Message, "status=404")
		require.Contains(t, entries[0].Message, "Internal API error")
		require.Contains(t, entries[0].Message, "correlation_id=")
	})
}

func testErrorMessage(t *testing.T, client *GitlabNetClient) {
	t.Run("Error with message for GET", func(t *testing.T) {
		response, err := client.Get(context.Background(), "/error")
		require.EqualError(t, err, "Don't do that")
		require.Nil(t, response)
	})

	t.Run("Error with message for POST", func(t *testing.T) {
		response, err := client.Post(context.Background(), "/error", map[string]string{})
		require.EqualError(t, err, "Don't do that")
		require.Nil(t, response)
	})
}

func testBrokenRequest(t *testing.T, client *GitlabNetClient) {
	t.Run("Broken request for GET", func(t *testing.T) {
		hook := testhelper.SetupLogger()

		response, err := client.Get(context.Background(), "/broken")
		require.EqualError(t, err, "Internal API unreachable")
		require.Nil(t, response)

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		require.Equal(t, 1, len(entries))
		require.Equal(t, logrus.InfoLevel, entries[0].Level)
		require.Contains(t, entries[0].Message, "method=GET")
		require.NotContains(t, entries[0].Message, "status=")
		require.Contains(t, entries[0].Message, "Internal API unreachable")
		require.Contains(t, entries[0].Message, "correlation_id=")
	})

	t.Run("Broken request for POST", func(t *testing.T) {
		hook := testhelper.SetupLogger()

		response, err := client.Post(context.Background(), "/broken", map[string]string{})
		require.EqualError(t, err, "Internal API unreachable")
		require.Nil(t, response)

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		require.Equal(t, 1, len(entries))
		require.Equal(t, logrus.InfoLevel, entries[0].Level)
		require.Contains(t, entries[0].Message, "method=POST")
		require.NotContains(t, entries[0].Message, "status=")
		require.Contains(t, entries[0].Message, "Internal API unreachable")
		require.Contains(t, entries[0].Message, "correlation_id=")
	})
}

func testAuthenticationHeader(t *testing.T, client *GitlabNetClient) {
	t.Run("Authentication headers for GET", func(t *testing.T) {
		response, err := client.Get(context.Background(), "/auth")
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)

		header, err := base64.StdEncoding.DecodeString(string(responseBody))
		require.NoError(t, err)
		require.Equal(t, "sssh, it's a secret", string(header))
	})

	t.Run("Authentication headers for POST", func(t *testing.T) {
		response, err := client.Post(context.Background(), "/auth", map[string]string{})
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)

		header, err := base64.StdEncoding.DecodeString(string(responseBody))
		require.NoError(t, err)
		require.Equal(t, "sssh, it's a secret", string(header))
	})
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

				b, err := ioutil.ReadAll(r.Body)
				defer r.Body.Close()

				require.NoError(t, err)

				fmt.Fprint(w, "Echo: "+string(b))
			},
		},
		{
			Path: "/api/v4/internal/auth",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, r.Header.Get(secretHeaderName))
			},
		},
		{
			Path: "/api/v4/internal/error",
			Handler: func(w http.ResponseWriter, r *http.Request) {
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
			Handler: func(w http.ResponseWriter, r *http.Request) {
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

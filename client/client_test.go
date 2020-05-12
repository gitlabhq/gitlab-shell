package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

func TestClients(t *testing.T) {
	testDirCleanup, err := testhelper.PrepareTestRootDir()
	require.NoError(t, err)
	defer testDirCleanup()

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

	testCases := []struct {
		desc   string
		caFile string
		server func(*testing.T, []testserver.TestRequestHandler) (string, func())
	}{
		{
			desc:   "Socket client",
			server: testserver.StartSocketHttpServer,
		},
		{
			desc:   "Http client",
			server: testserver.StartHttpServer,
		},
		{
			desc:   "Https client",
			caFile: path.Join(testhelper.TestRoot, "certs/valid/server.crt"),
			server: testserver.StartHttpsServer,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			url, cleanup := tc.server(t, requests)
			defer cleanup()

			secret := "sssh, it's a secret"

			httpClient := NewHTTPClient(url, tc.caFile, "", false, 1)

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
		response, err := client.Get("/hello")
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		assert.NoError(t, err)
		assert.Equal(t, string(responseBody), "Hello")

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, logrus.InfoLevel, entries[0].Level)
		assert.Contains(t, entries[0].Message, "method=GET")
		assert.Contains(t, entries[0].Message, "status=200")
		assert.Contains(t, entries[0].Message, "Finished HTTP request")
	})
}

func testSuccessfulPost(t *testing.T, client *GitlabNetClient) {
	t.Run("Successful Post", func(t *testing.T) {
		hook := testhelper.SetupLogger()
		data := map[string]string{"key": "value"}

		response, err := client.Post("/post_endpoint", data)
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		assert.NoError(t, err)
		assert.Equal(t, "Echo: {\"key\":\"value\"}", string(responseBody))

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, logrus.InfoLevel, entries[0].Level)
		assert.Contains(t, entries[0].Message, "method=POST")
		assert.Contains(t, entries[0].Message, "status=200")
		assert.Contains(t, entries[0].Message, "Finished HTTP request")
	})
}

func testMissing(t *testing.T, client *GitlabNetClient) {
	t.Run("Missing error for GET", func(t *testing.T) {
		hook := testhelper.SetupLogger()
		response, err := client.Get("/missing")
		assert.EqualError(t, err, "Internal API error (404)")
		assert.Nil(t, response)

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, logrus.InfoLevel, entries[0].Level)
		assert.Contains(t, entries[0].Message, "method=GET")
		assert.Contains(t, entries[0].Message, "status=404")
		assert.Contains(t, entries[0].Message, "Internal API error")
	})

	t.Run("Missing error for POST", func(t *testing.T) {
		hook := testhelper.SetupLogger()
		response, err := client.Post("/missing", map[string]string{})
		assert.EqualError(t, err, "Internal API error (404)")
		assert.Nil(t, response)

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, logrus.InfoLevel, entries[0].Level)
		assert.Contains(t, entries[0].Message, "method=POST")
		assert.Contains(t, entries[0].Message, "status=404")
		assert.Contains(t, entries[0].Message, "Internal API error")
	})
}

func testErrorMessage(t *testing.T, client *GitlabNetClient) {
	t.Run("Error with message for GET", func(t *testing.T) {
		response, err := client.Get("/error")
		assert.EqualError(t, err, "Don't do that")
		assert.Nil(t, response)
	})

	t.Run("Error with message for POST", func(t *testing.T) {
		response, err := client.Post("/error", map[string]string{})
		assert.EqualError(t, err, "Don't do that")
		assert.Nil(t, response)
	})
}

func testBrokenRequest(t *testing.T, client *GitlabNetClient) {
	t.Run("Broken request for GET", func(t *testing.T) {
		hook := testhelper.SetupLogger()

		response, err := client.Get("/broken")
		assert.EqualError(t, err, "Internal API unreachable")
		assert.Nil(t, response)

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, logrus.InfoLevel, entries[0].Level)
		assert.Contains(t, entries[0].Message, "method=GET")
		assert.NotContains(t, entries[0].Message, "status=")
		assert.Contains(t, entries[0].Message, "Internal API unreachable")
	})

	t.Run("Broken request for POST", func(t *testing.T) {
		hook := testhelper.SetupLogger()

		response, err := client.Post("/broken", map[string]string{})
		assert.EqualError(t, err, "Internal API unreachable")
		assert.Nil(t, response)

		require.True(t, testhelper.WaitForLogEvent(hook))
		entries := hook.AllEntries()
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, logrus.InfoLevel, entries[0].Level)
		assert.Contains(t, entries[0].Message, "method=POST")
		assert.NotContains(t, entries[0].Message, "status=")
		assert.Contains(t, entries[0].Message, "Internal API unreachable")
	})
}

func testAuthenticationHeader(t *testing.T, client *GitlabNetClient) {
	t.Run("Authentication headers for GET", func(t *testing.T) {
		response, err := client.Get("/auth")
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)

		header, err := base64.StdEncoding.DecodeString(string(responseBody))
		require.NoError(t, err)
		assert.Equal(t, "sssh, it's a secret", string(header))
	})

	t.Run("Authentication headers for POST", func(t *testing.T) {
		response, err := client.Post("/auth", map[string]string{})
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)

		header, err := base64.StdEncoding.DecodeString(string(responseBody))
		require.NoError(t, err)
		assert.Equal(t, "sssh, it's a secret", string(header))
	})
}

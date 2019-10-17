package gitlabnet

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
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
		config *config.Config
		server func(*testing.T, []testserver.TestRequestHandler) (string, func())
	}{
		{
			desc:   "Socket client",
			config: &config.Config{},
			server: testserver.StartSocketHttpServer,
		},
		{
			desc:   "Http client",
			config: &config.Config{},
			server: testserver.StartHttpServer,
		},
		{
			desc: "Https client",
			config: &config.Config{
				HttpSettings: config.HttpSettingsConfig{CaFile: path.Join(testhelper.TestRoot, "certs/valid/server.crt")},
			},
			server: testserver.StartHttpsServer,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			url, cleanup := tc.server(t, requests)
			defer cleanup()

			tc.config.GitlabUrl = url
			tc.config.Secret = "sssh, it's a secret"

			client, err := GetClient(tc.config)
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

func testSuccessfulGet(t *testing.T, client *GitlabClient) {
	t.Run("Successful get", func(t *testing.T) {
		response, err := client.Get("/hello")
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		assert.NoError(t, err)
		assert.Equal(t, string(responseBody), "Hello")
	})
}

func testSuccessfulPost(t *testing.T, client *GitlabClient) {
	t.Run("Successful Post", func(t *testing.T) {
		data := map[string]string{"key": "value"}

		response, err := client.Post("/post_endpoint", data)
		require.NoError(t, err)
		require.NotNil(t, response)

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		assert.NoError(t, err)
		assert.Equal(t, "Echo: {\"key\":\"value\"}", string(responseBody))
	})
}

func testMissing(t *testing.T, client *GitlabClient) {
	t.Run("Missing error for GET", func(t *testing.T) {
		response, err := client.Get("/missing")
		assert.EqualError(t, err, "Internal API error (404)")
		assert.Nil(t, response)
	})

	t.Run("Missing error for POST", func(t *testing.T) {
		response, err := client.Post("/missing", map[string]string{})
		assert.EqualError(t, err, "Internal API error (404)")
		assert.Nil(t, response)
	})
}

func testErrorMessage(t *testing.T, client *GitlabClient) {
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

func testBrokenRequest(t *testing.T, client *GitlabClient) {
	t.Run("Broken request for GET", func(t *testing.T) {
		response, err := client.Get("/broken")
		assert.EqualError(t, err, "Internal API unreachable")
		assert.Nil(t, response)
	})

	t.Run("Broken request for POST", func(t *testing.T) {
		response, err := client.Post("/broken", map[string]string{})
		assert.EqualError(t, err, "Internal API unreachable")
		assert.Nil(t, response)
	})
}

func testAuthenticationHeader(t *testing.T, client *GitlabClient) {
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

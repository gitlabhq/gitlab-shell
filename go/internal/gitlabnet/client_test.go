package gitlabnet

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/testserver"
)

func TestClients(t *testing.T) {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/hello",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "Hello")
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
	testConfig := &config.Config{GitlabUrl: "http+unix://" + testserver.TestSocket, Secret: "sssh, it's a secret"}

	testCases := []struct {
		desc   string
		client GitlabClient
		server func([]testserver.TestRequestHandler) (func(), error)
	}{
		{
			desc:   "Socket client",
			client: buildSocketClient(testConfig),
			server: testserver.StartSocketHttpServer,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cleanup, err := tc.server(requests)
			defer cleanup()
			require.NoError(t, err)

			testBrokenRequest(t, tc.client)
			testSuccessfulGet(t, tc.client)
			testMissing(t, tc.client)
			testErrorMessage(t, tc.client)
			testAuthenticationHeader(t, tc.client)
		})
	}
}

func testSuccessfulGet(t *testing.T, client GitlabClient) {
	t.Run("Successful get", func(t *testing.T) {
		response, err := client.Get("/hello")
		defer response.Body.Close()

		require.NoError(t, err)
		require.NotNil(t, response)

		responseBody, err := ioutil.ReadAll(response.Body)
		assert.NoError(t, err)
		assert.Equal(t, string(responseBody), "Hello")
	})
}

func testMissing(t *testing.T, client GitlabClient) {
	t.Run("Missing error", func(t *testing.T) {
		response, err := client.Get("/missing")
		assert.EqualError(t, err, "Internal API error (404)")
		assert.Nil(t, response)
	})
}

func testErrorMessage(t *testing.T, client GitlabClient) {
	t.Run("Error with message", func(t *testing.T) {
		response, err := client.Get("/error")
		assert.EqualError(t, err, "Don't do that")
		assert.Nil(t, response)
	})
}

func testBrokenRequest(t *testing.T, client GitlabClient) {
	t.Run("Broken request", func(t *testing.T) {
		response, err := client.Get("/broken")
		assert.EqualError(t, err, "Internal API unreachable")
		assert.Nil(t, response)
	})
}

func testAuthenticationHeader(t *testing.T, client GitlabClient) {
	t.Run("Authentication headers", func(t *testing.T) {
		response, err := client.Get("/auth")
		defer response.Body.Close()

		require.NoError(t, err)
		require.NotNil(t, response)

		responseBody, err := ioutil.ReadAll(response.Body)
		require.NoError(t, err)

		header, err := base64.StdEncoding.DecodeString(string(responseBody))
		require.NoError(t, err)
		assert.Equal(t, "sssh, it's a secret", string(header))
	})
}

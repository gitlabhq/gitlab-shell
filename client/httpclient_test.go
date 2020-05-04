package client

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
)

func TestReadTimeout(t *testing.T) {
	expectedSeconds := uint64(300)

	client := NewHTTPClient("http://localhost:3000", "", "", false, expectedSeconds)

	require.NotNil(t, client)
	assert.Equal(t, time.Duration(expectedSeconds)*time.Second, client.Client.Timeout)
}

const (
	username = "basic_auth_user"
	password = "basic_auth_password"
)

func TestBasicAuthSettings(t *testing.T) {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/get_endpoint",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)

				fmt.Fprint(w, r.Header.Get("Authorization"))
			},
		},
		{
			Path: "/api/v4/internal/post_endpoint",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)

				fmt.Fprint(w, r.Header.Get("Authorization"))
			},
		},
	}

	client, cleanup := setup(t, username, password, requests)
	defer cleanup()

	response, err := client.Get("/get_endpoint")
	require.NoError(t, err)
	testBasicAuthHeaders(t, response)

	response, err = client.Post("/post_endpoint", nil)
	require.NoError(t, err)
	testBasicAuthHeaders(t, response)
}

func testBasicAuthHeaders(t *testing.T, response *http.Response) {
	defer response.Body.Close()

	require.NotNil(t, response)
	responseBody, err := ioutil.ReadAll(response.Body)
	assert.NoError(t, err)

	headerParts := strings.Split(string(responseBody), " ")
	assert.Equal(t, "Basic", headerParts[0])

	credentials, err := base64.StdEncoding.DecodeString(headerParts[1])
	require.NoError(t, err)

	assert.Equal(t, username+":"+password, string(credentials))
}

func TestEmptyBasicAuthSettings(t *testing.T) {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/empty_basic_auth",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "", r.Header.Get("Authorization"))
			},
		},
	}

	client, cleanup := setup(t, "", "", requests)
	defer cleanup()

	_, err := client.Get("/empty_basic_auth")
	require.NoError(t, err)
}

func setup(t *testing.T, username, password string, requests []testserver.TestRequestHandler) (*GitlabNetClient, func()) {
	url, cleanup := testserver.StartHttpServer(t, requests)

	httpClient := NewHTTPClient(url, "", "", false, 1)

	client, err := NewGitlabNetClient(username, password, "", httpClient)
	require.NoError(t, err)

	return client, cleanup
}

package gitlabnet

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
)

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
	config := &config.Config{HttpSettings: config.HttpSettingsConfig{User: username, Password: password}}

	client, cleanup := setup(t, config, requests)
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

	client, cleanup := setup(t, &config.Config{}, requests)
	defer cleanup()

	_, err := client.Get("/empty_basic_auth")
	require.NoError(t, err)
}

func setup(t *testing.T, config *config.Config, requests []testserver.TestRequestHandler) (*GitlabClient, func()) {
	url, cleanup := testserver.StartHttpServer(t, requests)

	config.GitlabUrl = url
	client, err := GetClient(config)
	require.NoError(t, err)

	return client, cleanup
}

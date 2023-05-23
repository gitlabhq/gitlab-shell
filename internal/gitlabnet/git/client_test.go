package git

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	httpclient "gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
)

var customHeaders = map[string]string{
	"Authorization": "Bearer: token",
	"Header-One":    "Value-Two",
}

func TestInfoRefs(t *testing.T) {
	client := setup(t)

	for _, service := range []string{
		"git-receive-pack",
		"git-upload-pack",
		"git-archive-pack",
	} {
		response, err := client.InfoRefs(context.Background(), service)
		require.NoError(t, err)

		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		defer response.Body.Close()

		require.Equal(t, service, string(body))
	}
}

func TestReceivePack(t *testing.T) {
	client := setup(t)

	content := "content"
	response, err := client.ReceivePack(context.Background(), bytes.NewReader([]byte(content)))
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	require.Equal(t, "git-receive-pack: content", string(body))
}

func TestFailedHTTPRequest(t *testing.T) {
	client := &Client{
		Url:     testserver.StartHttpServer(t, []testserver.TestRequestHandler{}),
		Headers: customHeaders,
	}

	response, err := client.InfoRefs(context.Background(), "git-receive-pack")
	require.Nil(t, response)
	require.Error(t, err)

	var apiErr *httpclient.ApiError
	require.ErrorAs(t, err, &apiErr)
	require.EqualError(t, err, repoUnavailableErrMsg)
}

func setup(t *testing.T) *Client {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/info/refs",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, customHeaders["Authorization"], r.Header.Get("Authorization"))
				require.Equal(t, customHeaders["Header-One"], r.Header.Get("Header-One"))

				w.Write([]byte(r.URL.Query().Get("service")))
			},
		},
		{
			Path: "/git-receive-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, customHeaders["Authorization"], r.Header.Get("Authorization"))
				require.Equal(t, customHeaders["Header-One"], r.Header.Get("Header-One"))
				require.Equal(t, "application/x-git-receive-pack-request", r.Header.Get("Content-Type"))
				require.Equal(t, "application/x-git-receive-pack-result", r.Header.Get("Accept"))
				require.Equal(t, customHeaders["Header-One"], r.Header.Get("Header-One"))

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				defer r.Body.Close()

				w.Write([]byte("git-receive-pack: "))
				w.Write(body)
			},
		},
	}

	client := &Client{
		Url:     testserver.StartHttpServer(t, requests),
		Headers: customHeaders,
	}

	return client
}

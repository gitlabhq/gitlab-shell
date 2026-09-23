package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	httpclient "gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
)

const infoRefsPath = "/info/refs"

var customHeaders = map[string]string{
	"Authorization": "Bearer: token",
	"Header-One":    "Value-Two",
}
var refsBody = "0032want 0a53e9ddeaddad63ad106860237bbf53411d11a7\n"

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

func TestUploadPack(t *testing.T) {
	client := setup(t)

	response, err := client.UploadPack(context.Background(), bytes.NewReader([]byte(refsBody)))
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	require.Equal(t, "git-upload-pack: content", string(body))
}

func TestSSHUploadPack(t *testing.T) {
	client := setup(t)

	response, err := client.SSHUploadPack(context.Background(), bytes.NewReader([]byte(refsBody)))
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	require.Equal(t, "ssh-upload-pack: content", string(body))
}

func TestSSHReceivePack(t *testing.T) {
	client := setup(t)

	response, err := client.SSHReceivePack(context.Background(), bytes.NewReader([]byte(refsBody)))
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	require.Equal(t, "ssh-receive-pack: content", string(body))
}

func TestFailedHTTPRequest(t *testing.T) {
	requests := []testserver.TestRequestHandler{
		{
			Path: infoRefsPath,
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("You are not allowed to upload code."))
			},
		},
	}

	client := &Client{
		URL:     testserver.StartHTTPServer(t, requests),
		Headers: customHeaders,
	}

	response, err := client.InfoRefs(context.Background(), "git-receive-pack")
	require.Nil(t, response)
	require.Error(t, err)

	var apiErr *httpclient.APIError
	require.ErrorAs(t, err, &apiErr)
	require.EqualError(t, err, "You are not allowed to upload code.")

	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
}

func TestFailedErrorReadRequest(t *testing.T) {
	requests := []testserver.TestRequestHandler{
		{
			Path: infoRefsPath,
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				// Simulate a read error by saying Content-Length is larger than actual content.
				w.Header().Set("Content-Length", "1")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("test"))
			},
		},
	}

	client := &Client{
		URL:     testserver.StartHTTPServer(t, requests),
		Headers: customHeaders,
	}

	response, err := client.InfoRefs(context.Background(), "git-receive-pack")
	require.Nil(t, response)
	require.Error(t, err)

	var apiErr *httpclient.APIError
	require.ErrorAs(t, err, &apiErr)
	require.EqualError(t, err, repoUnavailableErrMsg)

	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
}

func TestHeaderFunc(t *testing.T) {
	var received []string
	url := testserver.StartHTTPServer(t, []testserver.TestRequestHandler{
		{
			Path: sshReceivePackPath,
			Handler: func(_ http.ResponseWriter, r *http.Request) {
				received = append(received, r.Header.Get("Authorization"))
				assert.Equal(t, "Value-Two", r.Header.Get("Header-One"))
			},
		},
	})

	calls := 0
	client := &Client{
		URL:     url,
		Headers: customHeaders,
		HeaderFunc: func() (map[string]string, error) {
			calls++
			return map[string]string{"Authorization": fmt.Sprintf("Bearer: token-%d", calls)}, nil
		},
	}

	for range 2 {
		response, err := client.SSHReceivePack(context.Background(), bytes.NewReader(nil))
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
	}

	require.Equal(t, []string{"Bearer: token-1", "Bearer: token-2"}, received)
}

func TestHeaderFuncError(t *testing.T) {
	headerErr := errors.New("header failed")
	body := &closeTrackingReader{}
	client := &Client{
		URL:        "http://127.0.0.1:0",
		HeaderFunc: func() (map[string]string, error) { return nil, headerErr },
	}

	response, err := client.SSHReceivePack(context.Background(), body) //nolint:bodyclose // no response on error
	require.Nil(t, response)
	require.ErrorIs(t, err, headerErr)
	require.True(t, body.closed)
}

type closeTrackingReader struct {
	closed bool
}

func (r *closeTrackingReader) Read(_ []byte) (int, error) { return 0, io.EOF }

func (r *closeTrackingReader) Close() error {
	r.closed = true
	return nil
}

func TestSSHErrorResponseWithOpenRequestBodyOverHTTP2(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("The project you were looking for could not be found."))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	originalHTTPClient := httpClient
	httpClient = &http.Client{Transport: httpclient.NewTransport(server.Client().Transport)}
	t.Cleanup(func() { httpClient = originalHTTPClient })

	client := &Client{URL: server.URL}

	testCases := []struct {
		desc      string
		requestFn func(context.Context, io.Reader) (*http.Response, error)
	}{
		{desc: "upload pack", requestFn: client.SSHUploadPack},
		{desc: "receive pack", requestFn: client.SSHReceivePack},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Like SSH stdin while git waits for the server's reply: open, silent,
			// and not closable by the transport.
			body, bodyWriter := io.Pipe()
			t.Cleanup(func() { bodyWriter.Close() })

			errCh := make(chan error, 1)
			go func() {
				response, err := tc.requestFn(context.Background(), io.NopCloser(body))
				if response != nil {
					response.Body.Close()
				}
				errCh <- err
			}()

			select {
			case err := <-errCh:
				require.EqualError(t, err, "The project you were looking for could not be found.")
			case <-time.After(5 * time.Second):
				t.Fatal("error not returned while the request body was open")
			}
		})
	}
}

func setup(t *testing.T) *Client {
	requests := []testserver.TestRequestHandler{
		{
			Path: infoRefsPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, customHeaders["Authorization"], r.Header.Get("Authorization"))
				assert.Equal(t, customHeaders["Header-One"], r.Header.Get("Header-One"))

				w.Write([]byte(r.URL.Query().Get("service")))
			},
		},
		{
			Path: "/git-receive-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, customHeaders["Authorization"], r.Header.Get("Authorization"))
				assert.Equal(t, customHeaders["Header-One"], r.Header.Get("Header-One"))
				assert.Equal(t, "application/x-git-receive-pack-request", r.Header.Get("Content-Type"))
				assert.Equal(t, "application/x-git-receive-pack-result", r.Header.Get("Accept"))

				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				w.Write([]byte("git-receive-pack: "))
				w.Write(body)
			},
		},
		{
			Path: "/git-upload-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, customHeaders["Authorization"], r.Header.Get("Authorization"))
				assert.Equal(t, customHeaders["Header-One"], r.Header.Get("Header-One"))
				assert.Equal(t, "application/x-git-upload-pack-request", r.Header.Get("Content-Type"))
				assert.Equal(t, "application/x-git-upload-pack-result", r.Header.Get("Accept"))

				_, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				w.Write([]byte("git-upload-pack: content"))
			},
		},
		{
			Path: sshUploadPackPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, customHeaders["Authorization"], r.Header.Get("Authorization"))
				assert.Equal(t, customHeaders["Header-One"], r.Header.Get("Header-One"))

				_, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				w.Write([]byte("ssh-upload-pack: content"))
			},
		},
		{
			Path: sshReceivePackPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, customHeaders["Authorization"], r.Header.Get("Authorization"))
				assert.Equal(t, customHeaders["Header-One"], r.Header.Get("Header-One"))

				_, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				w.Write([]byte("ssh-receive-pack: content"))
			},
		},
	}

	client := &Client{
		URL:     testserver.StartHTTPServer(t, requests),
		Headers: customHeaders,
	}

	return client
}

package lfstransfer

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const responseCancellationTimeout = 5 * time.Second

// retryablehttp retries other 5xx responses, which would rerun this single-use handler and hang or panic.
var nonRetryableStatusCodes = []int{http.StatusBadRequest, http.StatusNotImplemented}

func newStreamingServer(t *testing.T, statusCode int) (*httptest.Server, <-chan struct{}) {
	t.Helper()

	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()

		<-r.Context().Done()
		close(requestCanceled)
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})

	return server, requestCanceled
}

func requireSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(responseCancellationTimeout):
		require.Fail(t, "timed out waiting for signal")
	}
}

func TestNewAuthenticatedPostRequest(t *testing.T) {
	client := &Client{header: "custom-content-type", auth: "custom-authorization"}

	for _, tc := range []struct {
		desc    string
		url     string
		wantErr bool
	}{
		{
			desc: "authenticated request",
			url:  "https://example.com/locks",
		},
		{
			desc:    "invalid url",
			url:     "://example.com/locks",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			req, err := client.newAuthenticatedPostRequest(tc.url, strings.NewReader("body"))

			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, req)
				return
			}

			require.NoError(t, err)
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "https://example.com/locks", req.URL.String())
			require.Equal(t, "custom-content-type", req.Header.Get("Content-Type"))
			require.Len(t, req.Header.Values("Content-Type"), 1)
			require.Equal(t, "custom-authorization", req.Header.Get("Authorization"))
			require.Len(t, req.Header.Values("Authorization"), 1)
		})
	}
}

func TestBatchClosesErrorResponseBody(t *testing.T) {
	requireClosesErrorResponseBody(t, func(t *testing.T, client *Client, statusCode int) {
		response, err := client.Batch("download", nil, "", "")

		require.ErrorContains(t, err, fmt.Sprintf("response failed with status code: %d", statusCode))
		require.Nil(t, response)
	})
}

func TestGetObjectClosesErrorResponseBody(t *testing.T) {
	requireClosesErrorResponseBody(t, func(t *testing.T, client *Client, _ int) {
		body, size, err := client.GetObject("", client.href, nil)

		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, body)
		require.Zero(t, size)
	})
}

func requireClosesErrorResponseBody(t *testing.T, call func(*testing.T, *Client, int)) {
	t.Helper()

	for _, statusCode := range nonRetryableStatusCodes {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			server, requestCanceled := newStreamingServer(t, statusCode)
			client := &Client{href: server.URL, auth: "authorization", header: ClientHeader}

			call(t, client, statusCode)
			requireSignal(t, requestCanceled)
		})
	}
}

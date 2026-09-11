package lfstransfer

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	var connectionCount atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/objects/batch" {
			http.Error(w, fmt.Sprintf("unexpected request: %s %s", r.Method, r.URL.Path), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", ClientHeader)
		_, _ = w.Write([]byte(`{"objects":[]}`))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connectionCount.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	client, err := NewClient(nil, nil, server.URL, "authorization")
	require.NoError(t, err)
	require.NotNil(t, client.client)
	require.Equal(t, 3, client.client.RetryMax)
	require.Nil(t, client.client.Logger)

	for range 2 {
		response, err := client.Batch("download", nil, "", "sha256")
		require.NoError(t, err)
		require.Empty(t, response.Objects)
	}

	require.Equal(t, int32(1), connectionCount.Load())
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

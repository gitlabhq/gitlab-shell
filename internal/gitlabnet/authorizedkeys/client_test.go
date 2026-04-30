package authorizedkeys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tspb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology/topologytest"
)

var (
	requests []testserver.TestRequestHandler
)

func init() {
	requests = []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/authorized_keys",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Query().Get("key") {
				case "key":
					body := &Response{
						ID:  1,
						Key: "public-key",
					}
					json.NewEncoder(w).Encode(body)
				case "broken-message":
					w.WriteHeader(http.StatusForbidden)
					body := &client.ErrorResponse{
						Message: "Not allowed!",
					}
					json.NewEncoder(w).Encode(body)
				case "broken-json":
					w.Write([]byte("{ \"message\": \"broken json!\""))
				case "broken-empty":
					w.WriteHeader(http.StatusForbidden)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			},
		},
	}
}

func TestGetByKey(t *testing.T) {
	client := setup(t)

	result, err := client.GetByKey(context.Background(), "key")
	require.NoError(t, err)
	require.Equal(t, &Response{ID: 1, Key: "public-key"}, result)
}

func TestGetByKeyErrorResponses(t *testing.T) {
	client := setup(t)

	testCases := []struct {
		desc          string
		key           string
		expectedError string
	}{
		{
			desc:          "A response with an error message",
			key:           "broken-message",
			expectedError: "Not allowed!",
		},
		{
			desc:          "A response with bad JSON",
			key:           "broken-json",
			expectedError: "parsing failed",
		},
		{
			desc:          "A forbidden (403) response without message",
			key:           "broken-empty",
			expectedError: "Internal API error (403)",
		},
		{
			desc:          "A not found (404) response without message",
			key:           "not-found",
			expectedError: "Internal API error (404)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			resp, err := client.GetByKey(context.Background(), tc.key)

			require.EqualError(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

func TestGetByKeyWithTopologyService(t *testing.T) {
	t.Run("routes /authorized_keys to cell when TS returns PROXY", func(t *testing.T) {
		var cellReceived bool
		cellServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cellReceived = true
			assert.Contains(t, r.URL.Path, "authorized_keys")
			assert.Equal(t, "key", r.URL.Query().Get("key"))
			assert.NotEmpty(t, r.Header.Get("Gitlab-Shell-Api-Request"), "JWT header must be present on cell request")
			assert.NotEmpty(t, r.Header.Get("User-Agent"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id": 1, "key": "public-key"}`)
		}))
		t.Cleanup(cellServer.Close)

		var defaultReceived bool
		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defaultReceived = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id": 1, "key": "public-key"}`)
		}))
		t.Cleanup(defaultServer.Close)

		cellAddress := strings.TrimPrefix(cellServer.URL, "http://")
		mock := &topologytest.MockClassifyServer{
			Response: &tspb.ClassifyResponse{
				Action: tspb.ClassifyAction_PROXY,
				Proxy:  &tspb.ProxyInfo{Address: cellAddress},
			},
		}
		tsAddr, tsStop := topologytest.StartMockServer(t, mock)
		t.Cleanup(tsStop)

		tsClient := topology.NewClient(&topology.Config{
			Enabled: true,
			Address: tsAddr,
			Timeout: 5 * time.Second,
		})
		t.Cleanup(func() { _ = tsClient.Close() })

		cfg := &config.Config{
			GitlabURL:      defaultServer.URL,
			Secret:         "test-secret",
			TopologyClient: tsClient,
		}

		client, err := NewClient(cfg)
		require.NoError(t, err)

		result, err := client.GetByKey(context.Background(), "key")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.ID)

		require.True(t, cellReceived, "request should have been sent to the cell server")
		require.False(t, defaultReceived, "request should NOT have been sent to the default server")

		require.Equal(t, "key", mock.LastRequest.GetClaim().GetSshKey())
	})

	t.Run("falls back to default when TS is nil", func(t *testing.T) {
		var defaultReceived bool
		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defaultReceived = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id": 1, "key": "public-key"}`)
		}))
		t.Cleanup(defaultServer.Close)

		cfg := &config.Config{
			GitlabURL:      defaultServer.URL,
			Secret:         "test-secret",
			TopologyClient: nil,
		}

		client, err := NewClient(cfg)
		require.NoError(t, err)

		result, err := client.GetByKey(context.Background(), "key")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.ID)
		require.True(t, defaultReceived, "request should have been sent to the default server")
	})

	t.Run("falls back to default when TS returns error", func(t *testing.T) {
		var defaultReceived bool
		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defaultReceived = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id": 1, "key": "public-key"}`)
		}))
		t.Cleanup(defaultServer.Close)

		mock := &topologytest.MockClassifyServer{
			Err: fmt.Errorf("TS unavailable"),
		}
		tsAddr, tsStop := topologytest.StartMockServer(t, mock)
		t.Cleanup(tsStop)

		tsClient := topology.NewClient(&topology.Config{
			Enabled: true,
			Address: tsAddr,
			Timeout: 5 * time.Second,
		})
		t.Cleanup(func() { _ = tsClient.Close() })

		cfg := &config.Config{
			GitlabURL:      defaultServer.URL,
			Secret:         "test-secret",
			TopologyClient: tsClient,
		}

		client, err := NewClient(cfg)
		require.NoError(t, err)

		result, err := client.GetByKey(context.Background(), "key")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.ID)
		require.True(t, defaultReceived, "request should have fallen back to the default server")
	})

	t.Run("falls back to default when TS returns non-PROXY action", func(t *testing.T) {
		var defaultReceived bool
		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defaultReceived = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id": 1, "key": "public-key"}`)
		}))
		t.Cleanup(defaultServer.Close)

		mock := &topologytest.MockClassifyServer{
			Response: &tspb.ClassifyResponse{
				Action: tspb.ClassifyAction_ACTION_UNSPECIFIED,
			},
		}
		tsAddr, tsStop := topologytest.StartMockServer(t, mock)
		t.Cleanup(tsStop)

		tsClient := topology.NewClient(&topology.Config{
			Enabled: true,
			Address: tsAddr,
			Timeout: 5 * time.Second,
		})
		t.Cleanup(func() { _ = tsClient.Close() })

		cfg := &config.Config{
			GitlabURL:      defaultServer.URL,
			Secret:         "test-secret",
			TopologyClient: tsClient,
		}

		client, err := NewClient(cfg)
		require.NoError(t, err)

		result, err := client.GetByKey(context.Background(), "key")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.ID)
		require.True(t, defaultReceived, "request should have fallen back to the default server")
	})
}

func setup(t *testing.T) *Client {
	url := testserver.StartSocketHTTPServer(t, requests)

	client, err := NewClient(&config.Config{GitlabURL: url})
	require.NoError(t, err)

	return client
}

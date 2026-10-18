package authorizedcerts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
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

const (
	testSecret              = "test-secret"
	testAuthorizedCertsPath = "/api/v4/internal/authorized_certs"
)

var (
	requests []testserver.TestRequestHandler
)

func init() {
	requests = []testserver.TestRequestHandler{
		{
			Path: testAuthorizedCertsPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				switch key := r.URL.Query().Get("key"); key {
				case "key":
					body := &Response{
						Namespace: "group",
						Username:  r.URL.Query().Get("user_identifier"),
					}
					json.NewEncoder(w).Encode(body)
				case "instance-key":
					w.Write([]byte(`{ "success": true, "username": "instance-user", "instance": true }`))
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

	result, err := client.GetByKey(context.Background(), "user-id", "key", "")
	require.NoError(t, err)
	require.Equal(t, &Response{Namespace: "group", Username: "user-id"}, result)
}

func TestGetByKeyInstanceLevel(t *testing.T) {
	client := setup(t)

	result, err := client.GetByKey(context.Background(), "user-id", "instance-key", "")
	require.NoError(t, err)
	require.Equal(t, &Response{Username: "instance-user", Instance: true}, result)
}

func TestGetByKeyForwardsRemoteIP(t *testing.T) {
	const (
		ipv4 = "192.0.2.10"
		ipv6 = "2001:db8::1"
	)

	testCases := []struct {
		desc       string
		remoteAddr string
		expectedIP string
	}{
		{desc: "IPv4 with port", remoteAddr: ipv4 + ":50000", expectedIP: ipv4},
		{desc: "IPv6 with port", remoteAddr: "[" + ipv6 + "]:50000", expectedIP: ipv6},
		{desc: "bare IPv4", remoteAddr: ipv4, expectedIP: ipv4},
		{desc: "bare IPv6", remoteAddr: ipv6, expectedIP: ipv6},
		{desc: "zoned IPv6", remoteAddr: "fe80::1%eth0", expectedIP: "fe80::1%eth0"},
		{desc: "no address", remoteAddr: ""},
		{desc: "Unix socket path", remoteAddr: "/run/lb.sock"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			var query url.Values
			handlers := []testserver.TestRequestHandler{
				{
					Path: testAuthorizedCertsPath,
					Handler: func(w http.ResponseWriter, r *http.Request) {
						query = r.URL.Query()
						_, _ = w.Write([]byte(`{ "username": "user-id", "namespace": "group" }`))
					},
				},
			}

			client, err := NewClient(&config.Config{GitlabURL: testserver.StartSocketHTTPServer(t, handlers)})
			require.NoError(t, err)

			_, err = client.GetByKey(context.Background(), "user-id", "key", tc.remoteAddr)
			require.NoError(t, err)

			if tc.expectedIP == "" {
				require.False(t, query.Has("check_ip"), "check_ip must be omitted when the client address is unknown or invalid")
			} else {
				require.True(t, query.Has("check_ip"))
				require.Equal(t, tc.expectedIP, query.Get("check_ip"))
			}
		})
	}
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
			resp, err := client.GetByKey(context.Background(), "user-id", tc.key, "")

			require.EqualError(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

func TestGetByKeyRateLimited(t *testing.T) {
	var rateLimitedRequests atomic.Int32
	handlers := []testserver.TestRequestHandler{
		{
			Path: testAuthorizedCertsPath,
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				rateLimitedRequests.Add(1)
				// Rails' check_rate_limit! body; its object message doesn't decode into ErrorResponse.
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"message":{"error":"This endpoint has been requested too many times. Try again later."}}`))
			},
		},
	}

	c, err := NewClient(&config.Config{GitlabURL: testserver.StartSocketHTTPServer(t, handlers)})
	require.NoError(t, err)

	// A regressed client would sleep on Retry-After until this deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := c.GetByKey(ctx, "user-id", "key", "")

	require.EqualValues(t, 1, rateLimitedRequests.Load())
	require.Nil(t, resp)
	require.Equal(t, &client.APIError{
		Msg:        "Internal API error (429)",
		StatusCode: http.StatusTooManyRequests,
		System:     false,
	}, err)
}

func TestGetByKeyWithTopologyService(t *testing.T) {
	t.Run("routes /authorized_certs to cell when TS returns PROXY", func(t *testing.T) {
		var cellReceived bool
		cellServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cellReceived = true
			assert.Contains(t, r.URL.Path, "authorized_certs")
			assert.Equal(t, "fingerprint-value", r.URL.Query().Get("key"))
			assert.Equal(t, "user-id", r.URL.Query().Get("user_identifier"))
			assert.NotEmpty(t, r.Header.Get("Gitlab-Shell-Api-Request"), "JWT header must be present on cell request")
			assert.NotEmpty(t, r.Header.Get("User-Agent"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"username": "root", "namespace": "group"}`)
		}))
		t.Cleanup(cellServer.Close)

		var defaultReceived bool
		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defaultReceived = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"username": "root", "namespace": "group"}`)
		}))
		t.Cleanup(defaultServer.Close)

		client, mock := newRoutedClient(t, cellServer, defaultServer)

		result, err := client.GetByKey(context.Background(), "user-id", "fingerprint-value", "")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "root", result.Username)

		require.True(t, cellReceived, "request should have been sent to the cell server")
		require.False(t, defaultReceived, "request should NOT have been sent to the default server")

		require.Equal(t, "fingerprint-value", mock.LastClaim(t).GetSshKeyFingerprint())
	})

	t.Run("does not retry a 429 from the routed cell", func(t *testing.T) {
		var cellRequests atomic.Int32
		cellServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			cellRequests.Add(1)
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(cellServer.Close)

		defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			assert.Fail(t, "request should NOT have been sent to the default server")
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(defaultServer.Close)

		c, _ := newRoutedClient(t, cellServer, defaultServer)

		// A regressed client would sleep on Retry-After until this deadline.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		result, err := c.GetByKey(ctx, "user-id", "fingerprint-value", "")

		require.EqualValues(t, 1, cellRequests.Load())
		require.Nil(t, result)

		var apiErr *client.APIError
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
		require.False(t, apiErr.System)
	})

	t.Run("falls back to default", func(t *testing.T) {
		tests := []struct {
			name string
			mock *topologytest.MockClassifyServer // nil means TS not configured
		}{
			{
				name: "when TS is nil",
				mock: nil,
			},
			{
				name: "when TS returns error",
				mock: &topologytest.MockClassifyServer{
					Err: fmt.Errorf("TS unavailable"),
				},
			},
			{
				name: "when TS returns non-PROXY action",
				mock: &topologytest.MockClassifyServer{
					Response: &tspb.ClassifyResponse{
						Action: tspb.ClassifyAction_ACTION_UNSPECIFIED,
					},
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				var defaultReceived bool
				defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					defaultReceived = true
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"username": "root", "namespace": "group"}`)
				}))
				t.Cleanup(defaultServer.Close)

				cfg := &config.Config{
					GitlabURL: defaultServer.URL,
					Secret:    testSecret,
				}

				if tc.mock != nil {
					tsAddr, tsStop := topologytest.StartMockServer(t, tc.mock)
					t.Cleanup(tsStop)

					tsClient := topology.NewClient(&topology.Config{
						Enabled: true,
						Address: tsAddr,
						Timeout: 5 * time.Second,
					})
					t.Cleanup(func() { _ = tsClient.Close() })

					cfg.TopologyClient = tsClient
				}

				client, err := NewClient(cfg)
				require.NoError(t, err)

				result, err := client.GetByKey(context.Background(), "user-id", "fingerprint-value", "")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, "root", result.Username)
				require.True(t, defaultReceived, "request should have been sent to the default server")
			})
		}
	})
}

// newRoutedClient returns a Client whose Topology Service mock always routes
// (PROXY) to cellServer, with defaultServer as the instance's own GitlabURL.
func newRoutedClient(t *testing.T, cellServer, defaultServer *httptest.Server) (*Client, *topologytest.MockClassifyServer) {
	t.Helper()

	cell := topologytest.CellAddressWithBogusPort(t, cellServer, 1)

	mock := &topologytest.MockClassifyServer{
		Response: &tspb.ClassifyResponse{
			Action: tspb.ClassifyAction_PROXY,
			Proxy:  &tspb.ProxyInfo{Address: cell.TopologyAddress},
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

	c, err := NewClient(&config.Config{
		GitlabURL:      defaultServer.URL,
		Secret:         testSecret,
		TopologyClient: tsClient,
		TopologyService: topology.Config{
			Enabled:      true,
			CellEndpoint: topology.CellEndpointConfig{Scheme: "http", Port: cell.RealPort},
		},
	})
	require.NoError(t, err)

	return c, mock
}

func setup(t *testing.T) *Client {
	url := testserver.StartSocketHTTPServer(t, requests)

	client, err := NewClient(&config.Config{GitlabURL: url})
	require.NoError(t, err)

	return client
}

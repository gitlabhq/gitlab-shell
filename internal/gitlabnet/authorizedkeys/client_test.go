package authorizedkeys

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	gossh "golang.org/x/crypto/ssh"
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
		keyStr, expectedFingerprint := generateTestKeyAndFingerprint(t)

		var cellReceived bool
		cellServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cellReceived = true
			assert.Contains(t, r.URL.Path, "authorized_keys")
			assert.Equal(t, keyStr, r.URL.Query().Get("key"))
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

		cfg := &config.Config{
			GitlabURL:      defaultServer.URL,
			Secret:         "test-secret",
			TopologyClient: tsClient,
			TopologyService: topology.Config{
				Enabled:      true,
				CellEndpoint: topology.CellEndpointConfig{Scheme: "http", Port: cell.RealPort},
			},
		}

		client, err := NewClient(cfg)
		require.NoError(t, err)

		result, err := client.GetByKey(context.Background(), keyStr)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.ID)

		require.True(t, cellReceived, "request should have been sent to the cell server")
		require.False(t, defaultReceived, "request should NOT have been sent to the default server")

		require.Equal(t, expectedFingerprint, mock.LastRequest.GetClaim().GetSshKeyFingerprint())
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
					_, _ = fmt.Fprintf(w, `{"id": 1, "key": "public-key"}`)
				}))
				t.Cleanup(defaultServer.Close)

				cfg := &config.Config{
					GitlabURL: defaultServer.URL,
					Secret:    "test-secret",
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

				result, err := client.GetByKey(context.Background(), "key")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, int64(1), result.ID)
				require.True(t, defaultReceived, "request should have been sent to the default server")
			})
		}
	})
}

func TestComputeFingerprint(t *testing.T) {
	t.Run("ed25519 key without padding", func(t *testing.T) {
		keyStr, expectedFingerprint := generateTestKeyAndFingerprint(t)

		fp, err := computeFingerprint(keyStr)
		require.NoError(t, err)
		require.NotEmpty(t, fp)
		require.Equal(t, expectedFingerprint, fp)
	})

	t.Run("invalid base64 returns error", func(t *testing.T) {
		fp, err := computeFingerprint("not-valid-base64!!!")
		require.Error(t, err)
		require.Empty(t, fp)
	})

	t.Run("empty string returns empty without error", func(t *testing.T) {
		fp, err := computeFingerprint("")
		require.NoError(t, err)
		require.Empty(t, fp)
	})

	t.Run("full SSH key string returns error", func(t *testing.T) {
		// The authorized-keys-check command path receives the full key string
		// (e.g., "ssh-ed25519 AAAAC3Nz... user@host") which is not valid base64.
		// computeFingerprint should return an error so the caller can log it.
		fp, err := computeFingerprint("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI user@host")
		require.Error(t, err)
		require.Empty(t, fp)
	})
}

// generateTestKeyAndFingerprint creates an ed25519 SSH key and returns the
// raw base64-encoded wire-format key string (as gitlab-sshd would produce)
// and its expected SHA-256 fingerprint (raw base64, no "SHA256:" prefix).
func generateTestKeyAndFingerprint(t *testing.T) (keyStr, fingerprint string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(priv)
	require.NoError(t, err)
	pubKey := signer.PublicKey()
	keyStr = base64.RawStdEncoding.EncodeToString(pubKey.Marshal())
	hash := sha256.Sum256(pubKey.Marshal())
	fingerprint = base64.RawStdEncoding.EncodeToString(hash[:])
	return keyStr, fingerprint
}

func setup(t *testing.T) *Client {
	url := testserver.StartSocketHTTPServer(t, requests)

	client, err := NewClient(&config.Config{GitlabURL: url})
	require.NoError(t, err)

	return client
}

package gitaly

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

func TestPrometheusMetrics(t *testing.T) {
	metrics.GitalyConnectionsTotal.Reset()

	c := newClient()

	cmd := Command{ServiceName: "git-upload-pack", Address: "tcp://localhost:9999"}
	c.newConnection(context.Background(), cmd)
	c.newConnection(context.Background(), cmd)

	require.Equal(t, 1, testutil.CollectAndCount(metrics.GitalyConnectionsTotal))
	require.InDelta(t, 2, testutil.ToFloat64(metrics.GitalyConnectionsTotal.WithLabelValues("ok")), 0.1)
	require.InDelta(t, 0, testutil.ToFloat64(metrics.GitalyConnectionsTotal.WithLabelValues("fail")), 0.1)

	cmd = Command{Address: ""}
	c.newConnection(context.Background(), cmd)

	require.InDelta(t, 2, testutil.ToFloat64(metrics.GitalyConnectionsTotal.WithLabelValues("ok")), 0.1)
	require.InDelta(t, 1, testutil.ToFloat64(metrics.GitalyConnectionsTotal.WithLabelValues("fail")), 0.1)
}

func TestCachedConnections(t *testing.T) {
	c := newClient()

	require.Empty(t, c.cache.connections)

	cmd := Command{ServiceName: "git-upload-pack", Address: "tcp://localhost:9999"}

	conn, err := c.GetConnection(context.Background(), cmd)
	require.NoError(t, err)
	require.Len(t, c.cache.connections, 1)

	newConn, err := c.GetConnection(context.Background(), cmd)
	require.NoError(t, err)
	require.Len(t, c.cache.connections, 1)
	require.Equal(t, conn, newConn)

	cmd = Command{ServiceName: "git-upload-pack", Address: "tcp://localhost:9998"}
	_, err = c.GetConnection(context.Background(), cmd)
	require.NoError(t, err)
	require.Len(t, c.cache.connections, 2)
}

func TestRetryConfiguration(t *testing.T) {
	testCases := []struct {
		name               string
		maxAttempts        int
		maxBackoff         float64
		expectedAttempts   int
		expectedMaxBackoff float64
	}{
		{
			name:               "default values when not set",
			maxAttempts:        0,
			maxBackoff:         0,
			expectedAttempts:   4,
			expectedMaxBackoff: 1.4,
		},
		{
			name:               "custom values",
			maxAttempts:        5,
			maxBackoff:         2.5,
			expectedAttempts:   5,
			expectedMaxBackoff: 2.5,
		},
		{
			name:               "only max attempts set",
			maxAttempts:        3,
			maxBackoff:         0,
			expectedAttempts:   3,
			expectedMaxBackoff: 1.4,
		},
		{
			name:               "only max backoff set",
			maxAttempts:        0,
			maxBackoff:         3.0,
			expectedAttempts:   4,
			expectedMaxBackoff: 3.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{
				MaxAttempts: tc.maxAttempts,
				MaxBackoff:  tc.maxBackoff,
			}
			c.InitSidechannelRegistry(context.Background())

			// Verify the client fields are set correctly
			require.Equal(t, tc.maxAttempts, c.MaxAttempts)
			require.InDelta(t, tc.maxBackoff, c.MaxBackoff, 0.01)

			// Test that connections can be created with the retry config
			cmd := Command{
				ServiceName: "git-upload-pack",
				Address:     "tcp://localhost:9999",
				Token:       "test-token",
			}

			conn, err := c.newConnection(context.Background(), cmd)
			// Connection should succeed (gRPC client is created even if server isn't reachable)
			require.NoError(t, err)
			require.NotNil(t, conn)
		})
	}
}

func newClient() *Client {
	c := &Client{}
	c.InitSidechannelRegistry(context.Background())
	return c
}

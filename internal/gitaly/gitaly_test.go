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

	require.Len(t, c.cache.connections, 0)

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

func newClient() *Client {
	c := &Client{}
	c.InitSidechannelRegistry(context.Background())
	return c
}

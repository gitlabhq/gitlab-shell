package topology

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
	"gitlab.com/gitlab-org/labkit/correlation"
	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

func TestNewClient(t *testing.T) {
	t.Run("returns nil when disabled", func(t *testing.T) {
		require.Nil(t, NewClient(&Config{Enabled: false}))
	})

	t.Run("applies defaults and does not mutate original config", func(t *testing.T) {
		cfg := &Config{
			Enabled: true,
			Address: "localhost:9090",
		}

		client := NewClient(cfg)

		// Client created successfully
		require.NotNil(t, client)
		require.Equal(t, "localhost:9090", client.config.Address)

		// Defaults applied to client config
		require.Equal(t, DefaultTimeout, client.config.Timeout)
		require.Equal(t, "first_cell", client.config.ClassifyType)

		// Original config unchanged
		require.Zero(t, cfg.Timeout)
		require.Empty(t, cfg.ClassifyType)
		require.NotSame(t, cfg, client.config)
	})

	t.Run("preserves custom values", func(t *testing.T) {
		cfg := &Config{
			Enabled:      true,
			Address:      "localhost:9090",
			Timeout:      10 * time.Second,
			ClassifyType: "session_prefix",
		}

		client := NewClient(cfg)

		require.Equal(t, 10*time.Second, client.config.Timeout)
		require.Equal(t, "session_prefix", client.config.ClassifyType)
	})
}

func TestClient_Close(t *testing.T) {
	t.Run("closing client with no connection does not error", func(t *testing.T) {
		client := &Client{config: &Config{Enabled: true, Address: "localhost:9090"}}
		require.NoError(t, client.Close())
	})

	t.Run("closing client with active connection clears state and allows reconnection", func(t *testing.T) {
		addr, stop := startMockServer(t, &mockClassifyServer{})
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})

		ctx := correlation.ContextWithClientName(context.Background(), "gitlab-shell-tests")

		// Establish connection by making a request
		result, err := client.Classify(ctx, "test-value")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify connection is established
		require.NotNil(t, client.conn)
		require.NotNil(t, client.client)

		// Close the client
		require.NoError(t, client.Close())

		// Verify state is cleared
		require.Nil(t, client.conn)
		require.Nil(t, client.client)

		// Verify reconnection works
		result, err = client.Classify(ctx, "test-value-2")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, client.conn)

		// Clean up
		require.NoError(t, client.Close())
	})
}

func TestParseClassifyType(t *testing.T) {
	tests := []struct {
		input    string
		expected pb.ClassifyType
	}{
		{"first_cell", pb.ClassifyType_FIRST_CELL},
		{"session_prefix", pb.ClassifyType_SESSION_PREFIX},
		{"cell_id", pb.ClassifyType_CELL_ID},
		{"unknown", pb.ClassifyType_FIRST_CELL},
		{"", pb.ClassifyType_FIRST_CELL},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, parseClassifyType(tt.input))
		})
	}
}

// mockClassifyServer implements the ClassifyService for testing.
type mockClassifyServer struct {
	pb.UnimplementedClassifyServiceServer
	response *pb.ClassifyResponse
	err      error
}

func (m *mockClassifyServer) Classify(_ context.Context, _ *pb.ClassifyRequest) (*pb.ClassifyResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.response != nil {
		return m.response, nil
	}
	return &pb.ClassifyResponse{
		Action: pb.ClassifyAction_PROXY,
		Proxy:  &pb.ProxyInfo{Address: "cell-1.gitlab.com:443"},
	}, nil
}

// startMockServer starts a gRPC server with the given mock and returns the address.
func startMockServer(t *testing.T, mock *mockClassifyServer) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	pb.RegisterClassifyServiceServer(server, mock)

	go func() { _ = server.Serve(lis) }()

	return lis.Addr().String(), server.Stop
}

func TestClient_Classify(t *testing.T) {
	ctx := correlation.ContextWithClientName(context.Background(), "gitlab-shell-tests")

	t.Run("successful request returns proxy info", func(t *testing.T) {
		addr, stop := startMockServer(t, &mockClassifyServer{})
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		result, err := client.Classify(ctx, "test-value")

		require.NoError(t, err)
		require.Equal(t, pb.ClassifyAction_PROXY, result.GetAction())
		require.Equal(t, "cell-1.gitlab.com:443", result.GetProxy().GetAddress())
	})

	t.Run("server error is propagated", func(t *testing.T) {
		addr, stop := startMockServer(t, &mockClassifyServer{
			err: fmt.Errorf("internal server error"),
		})
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		result, err := client.Classify(ctx, "test-value")

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "internal server error")
	})

	t.Run("unreachable server returns error", func(t *testing.T) {
		client := NewClient(&Config{
			Enabled: true,
			Address: "localhost:1", // Port 1 is unlikely to have a server
			Timeout: 100 * time.Millisecond,
		})
		defer client.Close()

		result, err := client.Classify(ctx, "test-value")

		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestClient_ClassifyWithTLS(t *testing.T) {
	testRoot := testhelper.PrepareTestRootDir(t)

	testCertsDir := path.Join(testRoot, "certs", "valid")
	if _, err := os.Stat(testCertsDir); os.IsNotExist(err) {
		t.Skip("Test certificates not available")
	}

	serverCert, err := tls.LoadX509KeyPair(
		filepath.Join(testCertsDir, "server.crt"),
		filepath.Join(testCertsDir, "server.key"),
	)
	if err != nil {
		t.Skipf("Could not load server certificates: %v", err)
	}

	lis, err := tls.Listen("tcp", "localhost:0", &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2"},
	})
	require.NoError(t, err)

	server := grpc.NewServer()
	pb.RegisterClassifyServiceServer(server, &mockClassifyServer{})
	go func() { _ = server.Serve(lis) }()
	defer server.Stop()

	caCert, err := os.ReadFile(filepath.Join(testCertsDir, "server.crt"))
	require.NoError(t, err)
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	client := NewClient(&Config{
		Enabled: true,
		Address: lis.Addr().String(),
		Timeout: 5 * time.Second,
		TLS: TLSConfig{
			Enabled:    true,
			CAFile:     filepath.Join(testCertsDir, "server.crt"),
			ServerName: "localhost",
		},
	})
	defer client.Close()

	ctx := correlation.ContextWithClientName(context.Background(), "gitlab-shell-tests")
	result, err := client.Classify(ctx, "test-value")

	require.NoError(t, err)
	require.Equal(t, pb.ClassifyAction_PROXY, result.GetAction())
}

func TestBuildTLSCredentials(t *testing.T) {
	t.Run("disabled TLS returns nil", func(t *testing.T) {
		creds, err := buildTLSCredentials(&Config{TLS: TLSConfig{Enabled: false}})
		require.NoError(t, err)
		require.Nil(t, creds)
	})

	t.Run("enabled TLS returns credentials", func(t *testing.T) {
		creds, err := buildTLSCredentials(&Config{
			TLS: TLSConfig{Enabled: true, InsecureSkipVerify: true},
		})
		require.NoError(t, err)
		require.NotNil(t, creds)
		require.Equal(t, "tls", creds.Info().SecurityProtocol)
	})

	t.Run("invalid CA file returns error", func(t *testing.T) {
		creds, err := buildTLSCredentials(&Config{
			TLS: TLSConfig{Enabled: true, CAFile: "/nonexistent/ca.crt"},
		})
		require.Error(t, err)
		require.Nil(t, creds)
	})

	t.Run("invalid client cert returns error", func(t *testing.T) {
		creds, err := buildTLSCredentials(&Config{
			TLS: TLSConfig{Enabled: true, CertFile: "/nonexistent/client.crt", KeyFile: "/nonexistent/client.key"},
		})
		require.Error(t, err)
		require.Nil(t, creds)
	})
}

func TestPrometheusMetrics(t *testing.T) {
	metrics.TopologyConnectionsTotal.Reset()
	metrics.TopologyRequestsTotal.Reset()

	// Successful request
	addr, stop := startMockServer(t, &mockClassifyServer{})
	defer stop()

	client := NewClient(&Config{Enabled: true, Address: addr, Timeout: 5 * time.Second})
	defer client.Close()

	_, err := client.Classify(context.Background(), "test-value")
	require.NoError(t, err)

	require.InDelta(t, 1, testutil.ToFloat64(metrics.TopologyConnectionsTotal.WithLabelValues("ok")), 0.1)
	require.InDelta(t, 1, testutil.ToFloat64(metrics.TopologyRequestsTotal.WithLabelValues("ok")), 0.1)
	require.Equal(t, 1, testutil.CollectAndCount(metrics.TopologyRequestDurationSeconds))

	// Failed request (server error)
	addrFail, stopFail := startMockServer(t, &mockClassifyServer{err: fmt.Errorf("error")})
	defer stopFail()

	clientFail := NewClient(&Config{Enabled: true, Address: addrFail, Timeout: 5 * time.Second})
	defer clientFail.Close()

	_, err = clientFail.Classify(context.Background(), "test-value")
	require.Error(t, err)

	require.InDelta(t, 2, testutil.ToFloat64(metrics.TopologyConnectionsTotal.WithLabelValues("ok")), 0.1)
	require.InDelta(t, 1, testutil.ToFloat64(metrics.TopologyRequestsTotal.WithLabelValues("fail")), 0.1)
}

package topology

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	types_proto "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto/types/v1"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology/topologytest"
	"gitlab.com/gitlab-org/labkit/correlation"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const localhostAddr = "localhost:9090"

func TestNewClient(t *testing.T) {
	t.Run("returns nil when disabled", func(t *testing.T) {
		require.Nil(t, NewClient(&Config{Enabled: false}))
	})

	t.Run("applies defaults and does not mutate original config", func(t *testing.T) {
		cfg := &Config{
			Enabled: true,
			Address: localhostAddr,
		}

		client := NewClient(cfg)

		// Client created successfully
		require.NotNil(t, client)
		require.Equal(t, localhostAddr, client.config.Address)

		// Defaults applied to client config
		require.Equal(t, DefaultTimeout, client.config.Timeout)

		// Original config unchanged
		require.Zero(t, cfg.Timeout)
		require.NotSame(t, cfg, client.config)
	})

	t.Run("preserves custom values", func(t *testing.T) {
		cfg := &Config{
			Enabled: true,
			Address: localhostAddr,
			Timeout: 10 * time.Second,
		}

		client := NewClient(cfg)

		require.Equal(t, 10*time.Second, client.config.Timeout)
	})
}

func TestClient_Close(t *testing.T) {
	t.Run("closing client with no connection does not error", func(t *testing.T) {
		client := &Client{config: &Config{Enabled: true, Address: localhostAddr}}
		require.NoError(t, client.Close())
	})

	t.Run("closing client with active connection clears state and allows reconnection", func(t *testing.T) {
		addr, stop := topologytest.StartMockServer(t, &topologytest.MockClassifyServer{})
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})

		ctx := correlation.ContextWithClientName(context.Background(), "gitlab-shell-tests")

		// Establish connection by making a request
		result, err := client.Classify(ctx, RouteClaim("test-value"))
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
		result, err = client.Classify(ctx, RouteClaim("test-value-2"))
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, client.conn)

		// Clean up
		require.NoError(t, client.Close())
	})
}

func TestClient_Classify(t *testing.T) {
	ctx := correlation.ContextWithClientName(context.Background(), "gitlab-shell-tests")

	t.Run("successful route claim returns proxy info", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		claim := RouteClaim("my-group/my-project")
		result, err := client.Classify(ctx, claim)

		require.NoError(t, err)
		require.Equal(t, pb.ClassifyAction_PROXY, result.GetAction())
		require.Equal(t, "cell-1.gitlab.com:443", result.GetProxy().GetAddress())

		// Verify the request was constructed correctly. The claim must be sent
		// as a single-element `claims` fallback chain: the Topology Service
		// removed the legacy singular `claim` field, and an empty chain makes
		// the server fall back to type/value classification and reject the
		// request with `InvalidArgument: invalid type: "UNSPECIFIED"`.
		require.Len(t, mock.LastRequest.GetClaims(), 1)
		require.Equal(t, "my-group/my-project", mock.LastClaim().GetRoute())
		require.Equal(t, pb.ClassifyType_UNSPECIFIED, mock.LastRequest.GetType())
		require.Empty(t, mock.LastRequest.GetValue())
	})

	t.Run("successful SSH key claim returns proxy info", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		claim := SSHKeyClaim("ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ")
		result, err := client.Classify(ctx, claim)

		require.NoError(t, err)
		require.Equal(t, pb.ClassifyAction_PROXY, result.GetAction())
		require.Equal(t, "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ", mock.LastClaim().GetSshKey())
	})

	t.Run("successful project ID claim returns proxy info", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		claim := ProjectIDClaim(42)
		result, err := client.Classify(ctx, claim)

		require.NoError(t, err)
		require.Equal(t, pb.ClassifyAction_PROXY, result.GetAction())
		require.Equal(t, int64(42), mock.LastClaim().GetProjectId())
	})

	// Regression test for the client/server proto skew that made every
	// Classify call fail with `InvalidArgument: invalid type: "UNSPECIFIED"`
	// on GitLab.com staging. The Topology Service removed the singular `claim`
	// field (reserved 4) in favor of `repeated claims = 5`; a client that
	// still populated the old field sent an effectively empty request, so the
	// server fell through to type/value classification with an unset type.
	//
	// This asserts the wire contract the server actually relies on: `claims`
	// is populated and `type`/`value` are left at their zero values so the
	// claims path takes precedence.
	t.Run("sends the claim via the claims chain, not type/value", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		for name, claim := range map[string]*types_proto.Claim{
			"route":           RouteClaim("my-group"),
			"ssh_key":         SSHKeyClaim("ssh-rsa AAAAB3"),
			"ssh_fingerprint": SSHFingerprintClaim("W3THTJOKxMaZp0VIOrjVSBVDnFjyzVSMFGMLmSPcaGo"),
			"project_id":      ProjectIDClaim(42),
			"username":        UsernameClaim("jane.doe"),
		} {
			t.Run(name, func(t *testing.T) {
				_, err := client.Classify(ctx, claim)
				require.NoError(t, err)

				// The claims chain carries the claim, and is within the
				// server's 5-claim limit.
				claims := mock.LastRequest.GetClaims()
				require.Len(t, claims, 1)
				require.LessOrEqual(t, len(claims), 5)
				require.True(t, proto.Equal(claim, claims[0]),
					"claim must be sent unmodified in the claims chain")

				// type/value must stay unset so the server takes the claims
				// path; a populated `type` would change classification.
				require.Equal(t, pb.ClassifyType_UNSPECIFIED, mock.LastRequest.GetType())
				require.Empty(t, mock.LastRequest.GetValue())
			})
		}
	})

	t.Run("server error is propagated", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Err: fmt.Errorf("internal server error"),
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		claim := RouteClaim("test")
		result, err := client.Classify(ctx, claim)

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "internal server error")
	})

	t.Run("unreachable server returns error", func(t *testing.T) {
		client := NewClient(&Config{
			Enabled: true,
			Address: "localhost:1",
			Timeout: 100 * time.Millisecond,
		})
		defer client.Close()

		claim := RouteClaim("test")
		result, err := client.Classify(ctx, claim)

		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("nil claim returns error without calling server", func(t *testing.T) {
		client := NewClient(&Config{
			Enabled: true,
			Address: "localhost:1",
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		result, err := client.Classify(ctx, nil)

		require.Error(t, err)
		require.Nil(t, result)
		require.EqualError(t, err, "claim must not be nil")
	})

	t.Run("nil client returns error without panicking", func(t *testing.T) {
		// NewClient returns nil when the Topology Service is disabled, so
		// Classify must not panic on a nil receiver.
		client := NewClient(&Config{Enabled: false})
		require.Nil(t, client)

		result, err := client.Classify(ctx, RouteClaim("test"))

		require.Error(t, err)
		require.Nil(t, result)
		require.EqualError(t, err, "topology service is disabled")
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
	pb.RegisterClassifyServiceServer(server, &topologytest.MockClassifyServer{})
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
	claim := RouteClaim("test-value")
	result, err := client.Classify(ctx, claim)

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
	// TopologyRequestDurationSeconds (Histogram) does not support Reset().

	// Successful request
	addr, stop := topologytest.StartMockServer(t, &topologytest.MockClassifyServer{})
	defer stop()

	client := NewClient(&Config{Enabled: true, Address: addr, Timeout: 5 * time.Second})
	defer client.Close()

	_, err := client.Classify(context.Background(), RouteClaim("test-value"))
	require.NoError(t, err)

	require.InDelta(t, 1, testutil.ToFloat64(metrics.TopologyConnectionsTotal.WithLabelValues("ok")), 0)
	require.InDelta(t, 0, testutil.ToFloat64(metrics.TopologyConnectionsTotal.WithLabelValues("fail")), 0)
	require.InDelta(t, 1, testutil.ToFloat64(metrics.TopologyRequestsTotal.WithLabelValues("ok")), 0)
	require.Equal(t, 1, testutil.CollectAndCount(metrics.TopologyRequestDurationSeconds))

	// Failed request (server error)
	addrFail, stopFail := topologytest.StartMockServer(t, &topologytest.MockClassifyServer{Err: fmt.Errorf("error")})
	defer stopFail()

	clientFail := NewClient(&Config{Enabled: true, Address: addrFail, Timeout: 5 * time.Second})
	defer clientFail.Close()

	_, err = clientFail.Classify(context.Background(), RouteClaim("test-value"))
	require.Error(t, err)

	require.InDelta(t, 2, testutil.ToFloat64(metrics.TopologyConnectionsTotal.WithLabelValues("ok")), 0)
	require.InDelta(t, 0, testutil.ToFloat64(metrics.TopologyConnectionsTotal.WithLabelValues("fail")), 0)
	require.InDelta(t, 1, testutil.ToFloat64(metrics.TopologyRequestsTotal.WithLabelValues("fail")), 0)
}

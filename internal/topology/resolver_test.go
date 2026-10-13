package topology

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology/topologytest"
	"gitlab.com/gitlab-org/labkit/correlation"
	"gitlab.com/gitlab-org/labkit/v2/fields"
	"gitlab.com/gitlab-org/labkit/v2/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	groupNamespace = "group"
	cell2Addr      = "cell-2:8080"
	janeDoe        = "jane-doe"
)

func TestExtractTopLevelNamespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"group/project.git", groupNamespace},
		{"/group/project.git", groupNamespace},
		{"group/sub/project.git", groupNamespace},
		{"", ""},
		{groupNamespace, groupNamespace},
		{".git", ""},
		{"//group/project.git", groupNamespace},
		{"///group/project.git", groupNamespace},
		{"//group", groupNamespace},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
			require.Equal(t, tc.want, ExtractTopLevelNamespace(tc.input))
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name         string
		mock         *topologytest.MockClassifyServer
		cellEndpoint CellEndpointConfig
		expected     string
	}{
		{
			name: "PROXY response returns address with http prefix",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: cell2Addr},
				},
			},
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTP, Port: 8080},
			expected:     "http://cell-2:8080",
		},
		{
			name: "server error returns empty string",
			mock: &topologytest.MockClassifyServer{
				Err: fmt.Errorf("internal server error"),
			},
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTP, Port: 8080},
			expected:     "",
		},
		{
			name: "non-PROXY action returns empty string",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_ACTION_UNSPECIFIED,
				},
			},
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTP, Port: 8080},
			expected:     "",
		},
		{
			name: "PROXY response uses configured https scheme",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: cell2Addr},
				},
			},
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8080},
			expected:     "https://cell-2:8080",
		},
		{
			name: "PROXY response overrides TS port with configured port",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: cell2Addr},
				},
			},
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "https://cell-2:8181",
		},
		{
			name: "PROXY response with unusable address falls back to default host",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: "http://gitlab.com"},
				},
			},
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
		{
			name: "PROXY response with empty address falls back to default host",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: ""},
				},
			},
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr, stop := topologytest.StartMockServer(t, tc.mock)
			defer stop()

			client := NewClient(&Config{
				Enabled: true,
				Address: addr,
				Timeout: 5 * time.Second,
			})
			defer client.Close()

			resolver := NewResolver(client, tc.cellEndpoint)
			result := resolver.resolve(context.Background(), RouteClaim("my-group"))
			require.Equal(t, tc.expected, result)
		})
	}

	t.Run("nil client returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, CellEndpointConfig{})
		result := resolver.resolve(context.Background(), RouteClaim("my-group"))
		require.Empty(t, result)
	})

	t.Run("nil claim returns empty string", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
		result := resolver.resolve(context.Background(), nil)
		require.Empty(t, result)
	})
}

func TestBuildCellURL(t *testing.T) {
	tests := []struct {
		name         string
		address      string
		cellEndpoint CellEndpointConfig
		expected     string
	}{
		{
			name:         "address with port has port overridden",
			address:      "cell-7:8080",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "https://cell-7:8181",
		},
		{
			name:         "address without port gets configured port appended",
			address:      "cell-1",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTP, Port: 8181},
			expected:     "http://cell-1:8181",
		},
		{
			name:         "IPv6 literal with port is rejected",
			address:      "[::1]:8080",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
		{
			name:         "bare IPv6 is rejected",
			address:      "::1",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
		{
			name:         "bracketed IPv6 is rejected",
			address:      "[::1]",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
		{
			name:         "bracketed full IPv6 is rejected",
			address:      "[2001:db8::1]",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
		{
			name:         "http scheme selection",
			address:      "cell-2:9090",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTP, Port: 80},
			expected:     "http://cell-2:80",
		},
		{
			name:         "address with scheme is rejected",
			address:      "http://gitlab.com",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
		{
			name:         "address with scheme and port is rejected",
			address:      "http://gitlab.com:8080",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
		{
			name:         "address with empty host is rejected",
			address:      ":8080",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
		{
			name:         "address with path is rejected",
			address:      "gitlab.com/path",
			cellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
			expected:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := NewResolver(nil, tc.cellEndpoint)
			require.Equal(t, tc.expected, resolver.buildCellURL(tc.address))
		})
	}
}

func TestResolveNotFoundIsNotAFailure(t *testing.T) {
	// Most projects are not assigned to a cell, so the Topology Service answers
	// NotFound for them. That must stay quiet: a debug line, no warning, and no
	// "fail" sample, or the fallback would look like an outage at scale.
	metrics.TopologyRequestsTotal.Reset()

	mock := &topologytest.MockClassifyServer{
		Err: status.Errorf(codes.NotFound, "claim not found"),
	}
	addr, stop := topologytest.StartMockServer(t, mock)
	defer stop()

	client := NewClient(&Config{Enabled: true, Address: addr, Timeout: 5 * time.Second})
	defer client.Close()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := log.WithLogger(context.Background(), logger)

	resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
	require.Empty(t, resolver.resolve(ctx, RouteClaim("my-group")))

	require.Equal(t, 1, mock.CallCount, "NotFound is not retried")

	var levels []string
	for _, line := range bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n")) {
		var entry map[string]any
		require.NoError(t, json.Unmarshal(line, &entry))
		if msg, _ := entry["msg"].(string); strings.HasPrefix(msg, "Topology Service") {
			levels = append(levels, fmt.Sprintf("%s: %s", entry["level"], msg))
		}
	}
	require.Equal(t, []string{"DEBUG: Topology Service has no claim for this route, using default host"}, levels)

	require.InDelta(t, 1, testutil.ToFloat64(metrics.TopologyRequestsTotal.WithLabelValues("not_found")), 0)
	require.InDelta(t, 0, testutil.ToFloat64(metrics.TopologyRequestsTotal.WithLabelValues("fail")), 0)
	require.InDelta(t, 0, testutil.ToFloat64(metrics.TopologyRequestsTotal.WithLabelValues("ok")), 0)
}

func TestIsClaimNotFound(t *testing.T) {
	notFound := status.Errorf(codes.NotFound, "claim not found")

	require.True(t, IsClaimNotFound(notFound))
	require.True(t, IsClaimNotFound(backoff.Permanent(notFound)), "sees through the backoff wrapper used by resolve")
	require.False(t, IsClaimNotFound(nil))
	require.False(t, IsClaimNotFound(status.Errorf(codes.Unavailable, "down")))
	require.False(t, IsClaimNotFound(backoff.Permanent(status.Errorf(codes.PermissionDenied, "no"))))
	require.False(t, IsClaimNotFound(fmt.Errorf("plain error")))
}

func TestResolveLogsIncludeCorrelationID(t *testing.T) {
	const correlationID = "test-correlation-id"

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

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := correlation.ContextWithCorrelation(context.Background(), correlationID)
	extracted := correlation.ExtractFromContext(ctx)
	ctx = log.WithLogger(ctx, logger.With(slog.String(fields.CorrelationID, extracted)))

	resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
	result := resolver.resolve(ctx, RouteClaim("my-group"))
	require.Empty(t, result)

	require.Contains(t, buf.String(), "Topology Service classify failed after retries")

	var found bool
	for _, line := range bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n")) {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["msg"] == "Topology Service classify failed after retries, falling back to default host" {
			require.Equal(t, extracted, entry["correlation_id"])
			found = true
		}
	}
	require.True(t, found, "expected fallback warning log line to be emitted")
}

func TestResolveByRoute(t *testing.T) {
	t.Run("extracts namespace and resolves", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: "cell-1:8080"},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
		result := resolver.resolveByRoute(context.Background(), "group/project.git")
		require.Equal(t, "http://cell-1:8080", result)

		// Verify the claim sent to the server used just the top-level namespace
		require.Equal(t, groupNamespace, mock.LastClaim(t).GetRoute())
	})

	t.Run("empty repo path returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, CellEndpointConfig{})
		result := resolver.resolveByRoute(context.Background(), "")
		require.Empty(t, result)
	})

	t.Run("nil client returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, CellEndpointConfig{})
		result := resolver.resolveByRoute(context.Background(), "group/project.git")
		require.Empty(t, result)
	})

	t.Run("uses configured https scheme", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: "cell-1:8080"},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTPS, Port: 8080})
		result := resolver.resolveByRoute(context.Background(), "group/project.git")
		require.Equal(t, "https://cell-1:8080", result)
	})
}

func TestResolveRetry(t *testing.T) {
	tests := []struct {
		name        string
		mock        *topologytest.MockClassifyServer
		cancelCtx   bool
		expected    string
		expectCalls int
		maxCalls    bool // when true, use require.LessOrEqual instead of require.Equal for call count
	}{
		{
			name: "retries on transient error and succeeds",
			mock: &topologytest.MockClassifyServer{
				Err:             fmt.Errorf("transient"),
				ErrUntilAttempt: 2,
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: cell2Addr},
				},
			},
			expected:    "http://cell-2:8080",
			expectCalls: 2,
		},
		{
			name: "exhausts all retries and returns empty string",
			mock: &topologytest.MockClassifyServer{
				Err: fmt.Errorf("persistent failure"),
			},
			expected:    "",
			expectCalls: int(classifyMaxAttempts),
		},
		{
			name: "does not retry on successful non-PROXY response",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_ACTION_UNSPECIFIED,
				},
			},
			expected:    "",
			expectCalls: 1,
		},
		{
			name: "respects context cancellation during retry",
			mock: &topologytest.MockClassifyServer{
				Err: fmt.Errorf("fail"),
			},
			cancelCtx:   true,
			expected:    "",
			expectCalls: 1,
			maxCalls:    true,
		},
		{
			name: "does not retry on NotFound error",
			mock: &topologytest.MockClassifyServer{
				Err: status.Errorf(codes.NotFound, "claim not found"),
			},
			expected:    "",
			expectCalls: 1,
		},
		{
			name: "does not retry on PermissionDenied error",
			mock: &topologytest.MockClassifyServer{
				Err: status.Errorf(codes.PermissionDenied, "permission denied"),
			},
			expected:    "",
			expectCalls: 1,
		},
		{
			name: "does not retry on InvalidArgument error",
			mock: &topologytest.MockClassifyServer{
				Err: status.Errorf(codes.InvalidArgument, "invalid argument"),
			},
			expected:    "",
			expectCalls: 1,
		},
		{
			name: "retries on Unavailable error",
			mock: &topologytest.MockClassifyServer{
				Err: status.Errorf(codes.Unavailable, "service unavailable"),
			},
			expected:    "",
			expectCalls: int(classifyMaxAttempts),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr, stop := topologytest.StartMockServer(t, tc.mock)
			defer stop()

			client := NewClient(&Config{
				Enabled: true,
				Address: addr,
				Timeout: 5 * time.Second,
			})
			defer client.Close()

			ctx := context.Background()
			if tc.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
			result := resolver.resolve(ctx, RouteClaim("my-group"))
			require.Equal(t, tc.expected, result)

			if tc.maxCalls {
				require.LessOrEqual(t, tc.mock.CallCount, tc.expectCalls)
			} else {
				require.Equal(t, tc.expectCalls, tc.mock.CallCount)
			}
		})
	}
}

func TestResolveBySSHKey(t *testing.T) {
	t.Run("resolves cell address from SSH key", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: cell2Addr},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
		result := resolver.resolveBySSHKey(context.Background(), "ssh-rsa AAAAB3...")
		require.Equal(t, "http://cell-2:8080", result)

		// Verify the claim sent to the server used the SSH key
		require.Equal(t, "ssh-rsa AAAAB3...", mock.LastClaim(t).GetSshKey())
	})

	t.Run("empty key returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, CellEndpointConfig{})
		result := resolver.resolveBySSHKey(context.Background(), "")
		require.Empty(t, result)
	})

	t.Run("nil client returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, CellEndpointConfig{})
		result := resolver.resolveBySSHKey(context.Background(), "ssh-rsa AAAAB3...")
		require.Empty(t, result)
	})

	t.Run("uses configured https scheme", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: cell2Addr},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTPS, Port: 8080})
		result := resolver.resolveBySSHKey(context.Background(), "ssh-rsa AAAAB3...")
		require.Equal(t, "https://cell-2:8080", result)
	})
}

func TestResolveBySSHFingerprint(t *testing.T) {
	t.Run("resolves cell address from SSH fingerprint", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: cell2Addr},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
		result := resolver.resolveBySSHFingerprint(context.Background(), "W3THTJOKxMaZp0VIOrjVSBVDnFjyzVSMFGMLmSPcaGo")
		require.Equal(t, "http://cell-2:8080", result)

		require.Equal(t, "W3THTJOKxMaZp0VIOrjVSBVDnFjyzVSMFGMLmSPcaGo", mock.LastClaim(t).GetSshKeyFingerprint())
	})

	t.Run("empty fingerprint returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, CellEndpointConfig{})
		result := resolver.resolveBySSHFingerprint(context.Background(), "")
		require.Empty(t, result)
	})

	t.Run("nil client returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, CellEndpointConfig{})
		result := resolver.resolveBySSHFingerprint(context.Background(), "W3THTJOKxMaZp0VIOrjVSBVDnFjyzVSMFGMLmSPcaGo")
		require.Empty(t, result)
	})

	t.Run("uses configured https scheme", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: cell2Addr},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTPS, Port: 8080})
		result := resolver.resolveBySSHFingerprint(context.Background(), "W3THTJOKxMaZp0VIOrjVSBVDnFjyzVSMFGMLmSPcaGo")
		require.Equal(t, "https://cell-2:8080", result)
	})
}

func TestResolveByUserArgs(t *testing.T) {
	t.Run("resolves cell address from username", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: cell2Addr},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
		result := resolver.resolveByUserArgs(context.Background(), UserArgs{Username: janeDoe})
		require.Equal(t, "http://cell-2:8080", result)

		// Verify the claim sent to the server used the username
		require.Equal(t, janeDoe, mock.LastClaim(t).GetUsername())
	})

	fallbackTests := []struct {
		name string
		args UserArgs
	}{
		{"key ID only", UserArgs{KeyID: "123"}},
		{"krb5principal only", UserArgs{Krb5Principal: "user@REALM"}},
		{"empty args", UserArgs{}},
	}

	for _, tc := range fallbackTests {
		t.Run(tc.name+" returns empty string", func(t *testing.T) {
			resolver := NewResolver(nil, CellEndpointConfig{})
			result := resolver.resolveByUserArgs(context.Background(), tc.args)
			require.Empty(t, result)
		})
	}

	t.Run("non-nil resolver with no username does not call Topology Service", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: cell2Addr},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTP, Port: 8080})
		result := resolver.resolveByUserArgs(context.Background(), UserArgs{KeyID: "123"})
		require.Empty(t, result)

		// Verify the Topology Service was never contacted
		require.Equal(t, 0, mock.CallCount)
	})

	t.Run("nil resolver returns empty string", func(t *testing.T) {
		var resolver *Resolver
		result := resolver.resolveByUserArgs(context.Background(), UserArgs{Username: janeDoe})
		require.Empty(t, result)
	})

	t.Run("uses configured https scheme", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: cell2Addr},
			},
		}
		addr, stop := topologytest.StartMockServer(t, mock)
		defer stop()

		client := NewClient(&Config{
			Enabled: true,
			Address: addr,
			Timeout: 5 * time.Second,
		})
		defer client.Close()

		resolver := NewResolver(client, CellEndpointConfig{Scheme: schemeHTTPS, Port: 8080})
		result := resolver.resolveByUserArgs(context.Background(), UserArgs{Username: janeDoe})
		require.Equal(t, "https://cell-2:8080", result)
	})
}

func TestIsRetryableError(t *testing.T) {
	retryableCodes := []codes.Code{
		codes.Unavailable,
		codes.ResourceExhausted,
		codes.Aborted,
		codes.Internal,
		codes.DeadlineExceeded,
		codes.Unknown,
	}

	for _, code := range retryableCodes {
		t.Run(fmt.Sprintf("%s is retryable", code), func(t *testing.T) {
			require.True(t, isRetryableError(status.Errorf(code, "test")))
		})
	}

	nonRetryableCodes := []codes.Code{
		codes.NotFound,
		codes.PermissionDenied,
		codes.InvalidArgument,
		codes.Unauthenticated,
		codes.AlreadyExists,
		codes.FailedPrecondition,
		codes.Unimplemented,
		codes.OutOfRange,
		codes.DataLoss,
		codes.Canceled,
	}

	for _, code := range nonRetryableCodes {
		t.Run(fmt.Sprintf("%s is not retryable", code), func(t *testing.T) {
			require.False(t, isRetryableError(status.Errorf(code, "test")))
		})
	}

	t.Run("non-gRPC error is retryable", func(t *testing.T) {
		require.True(t, isRetryableError(fmt.Errorf("connection refused")))
	})
}

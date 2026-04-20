package topology

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology/topologytest"
)

func TestExtractTopLevelNamespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"group/project.git", "group"},
		{"/group/project.git", "group"},
		{"group/sub/project.git", "group"},
		{"", ""},
		{"group", "group"},
		{".git", ""},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
			require.Equal(t, tc.want, ExtractTopLevelNamespace(tc.input))
		})
	}
}

func TestResolve(t *testing.T) {
	t.Run("PROXY response returns address with http prefix", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: "cell-2:8080"},
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

		resolver := NewResolver(client)
		result := resolver.Resolve(context.Background(), RouteClaim("my-group"))
		require.Equal(t, "http://cell-2:8080", result)
	})

	t.Run("PROXY response with http prefix is not doubled", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: "http://cell-2:8080"},
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

		resolver := NewResolver(client)
		result := resolver.Resolve(context.Background(), RouteClaim("my-group"))
		require.Equal(t, "http://cell-2:8080", result)
	})

	t.Run("PROXY response with https prefix is preserved", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_PROXY,
				Proxy:  &pb.ProxyInfo{Address: "https://cell-2:8080"},
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

		resolver := NewResolver(client)
		result := resolver.Resolve(context.Background(), RouteClaim("my-group"))
		require.Equal(t, "https://cell-2:8080", result)
	})

	t.Run("server error returns empty string", func(t *testing.T) {
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

		resolver := NewResolver(client)
		result := resolver.Resolve(context.Background(), RouteClaim("my-group"))
		require.Empty(t, result)
	})

	t.Run("nil client returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil)
		result := resolver.Resolve(context.Background(), RouteClaim("my-group"))
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

		resolver := NewResolver(client)
		result := resolver.Resolve(context.Background(), nil)
		require.Empty(t, result)
	})

	t.Run("non-PROXY action returns empty string", func(t *testing.T) {
		mock := &topologytest.MockClassifyServer{
			Response: &pb.ClassifyResponse{
				Action: pb.ClassifyAction_ACTION_UNSPECIFIED,
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

		resolver := NewResolver(client)
		result := resolver.Resolve(context.Background(), RouteClaim("my-group"))
		require.Empty(t, result)
	})
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

		resolver := NewResolver(client)
		result := resolver.ResolveByRoute(context.Background(), "group/project.git")
		require.Equal(t, "http://cell-1:8080", result)

		// Verify the claim sent to the server used just the top-level namespace
		require.Equal(t, "group", mock.LastRequest.GetClaim().GetRoute())
	})

	t.Run("empty repo path returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil)
		result := resolver.ResolveByRoute(context.Background(), "")
		require.Empty(t, result)
	})

	t.Run("nil client returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil)
		result := resolver.ResolveByRoute(context.Background(), "group/project.git")
		require.Empty(t, result)
	})
}

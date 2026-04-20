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
	tests := []struct {
		name     string
		mock     *topologytest.MockClassifyServer
		expected string
	}{
		{
			name: "PROXY response returns address with http prefix",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: "cell-2:8080"},
				},
			},
			expected: "http://cell-2:8080",
		},
		{
			name: "PROXY response with http prefix is not doubled",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: "http://cell-2:8080"},
				},
			},
			expected: "http://cell-2:8080",
		},
		{
			name: "PROXY response with https prefix is preserved",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: "https://cell-2:8080"},
				},
			},
			expected: "https://cell-2:8080",
		},
		{
			name: "server error returns empty string",
			mock: &topologytest.MockClassifyServer{
				Err: fmt.Errorf("internal server error"),
			},
			expected: "",
		},
		{
			name: "non-PROXY action returns empty string",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_ACTION_UNSPECIFIED,
				},
			},
			expected: "",
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

			resolver := NewResolver(client)
			result := resolver.Resolve(context.Background(), RouteClaim("my-group"))
			require.Equal(t, tc.expected, result)
		})
	}

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

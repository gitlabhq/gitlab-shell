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
		{"//group/project.git", "group"},
		{"///group/project.git", "group"},
		{"//group", "group"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
			require.Equal(t, tc.want, ExtractTopLevelNamespace(tc.input))
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		mock      *topologytest.MockClassifyServer
		gitlabURL string
		expected  string
	}{
		{
			name: "PROXY response returns address with http prefix",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: "cell-2:8080"},
				},
			},
			gitlabURL: "http://localhost",
			expected:  "http://cell-2:8080",
		},
		{
			name: "server error returns empty string",
			mock: &topologytest.MockClassifyServer{
				Err: fmt.Errorf("internal server error"),
			},
			gitlabURL: "http://localhost",
			expected:  "",
		},
		{
			name: "non-PROXY action returns empty string",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_ACTION_UNSPECIFIED,
				},
			},
			gitlabURL: "http://localhost",
			expected:  "",
		},
		{
			name: "PROXY response with schemaless address uses https when GitLab URL is https",
			mock: &topologytest.MockClassifyServer{
				Response: &pb.ClassifyResponse{
					Action: pb.ClassifyAction_PROXY,
					Proxy:  &pb.ProxyInfo{Address: "cell-2:8080"},
				},
			},
			gitlabURL: "https://gitlab.example.com",
			expected:  "https://cell-2:8080",
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

			resolver := NewResolver(client, tc.gitlabURL)
			result := resolver.Resolve(context.Background(), RouteClaim("my-group"))
			require.Equal(t, tc.expected, result)
		})
	}

	t.Run("nil client returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, "http://localhost")
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

		resolver := NewResolver(client, "http://localhost")
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

		resolver := NewResolver(client, "http://localhost")
		result := resolver.ResolveByRoute(context.Background(), "group/project.git")
		require.Equal(t, "http://cell-1:8080", result)

		// Verify the claim sent to the server used just the top-level namespace
		require.Equal(t, "group", mock.LastRequest.GetClaim().GetRoute())
	})

	t.Run("empty repo path returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, "")
		result := resolver.ResolveByRoute(context.Background(), "")
		require.Empty(t, result)
	})

	t.Run("nil client returns empty string", func(t *testing.T) {
		resolver := NewResolver(nil, "http://localhost")
		result := resolver.ResolveByRoute(context.Background(), "group/project.git")
		require.Empty(t, result)
	})

	t.Run("uses https scheme from gitlabURL", func(t *testing.T) {
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

		resolver := NewResolver(client, "https://gitlab.example.com")
		result := resolver.ResolveByRoute(context.Background(), "group/project.git")
		require.Equal(t, "https://cell-1:8080", result)
	})
}

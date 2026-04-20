package topology

import (
	"context"
	"log/slog"
	"strings"

	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	types_proto "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto/types/v1"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

// Resolver queries the Topology Service to determine which cell should
// handle a request. It gracefully degrades: if the TS is disabled,
// unreachable, or returns an error, an empty string is returned and
// the caller should use the default host.
type Resolver struct {
	client *Client
	scheme string // "http" or "https", inferred from the original GitLab URL
}

// NewResolver creates a new Resolver. If client is nil (TS disabled),
// all Resolve calls return empty string immediately. The gitlabURL is used
// to infer the URL scheme (http or https) for schemaless addresses returned
// by the Topology Service.
func NewResolver(client *Client, gitlabURL string) *Resolver {
	scheme := "http" // default fallback
	if strings.HasPrefix(gitlabURL, "https://") {
		scheme = "https"
	}
	return &Resolver{client: client, scheme: scheme}
}

// Resolve queries the Topology Service with the given claim and returns
// the proxy address as an HTTP(S) URL string. The scheme is inferred from
// the original GitLab URL when the Topology Service returns a schemaless
// address. Returns empty string on any failure or when TS is not configured.
func (r *Resolver) Resolve(ctx context.Context, claim *types_proto.Claim) string {
	if r == nil || r.client == nil || claim == nil {
		return ""
	}

	resp, err := r.client.Classify(ctx, claim)
	if err != nil {
		slog.WarnContext(ctx, "Topology Service classify failed, falling back to default host",
			log.ErrorMessage(err.Error()))
		return ""
	}

	if resp.GetAction() == pb.ClassifyAction_PROXY && resp.GetProxy() != nil {
		address := resp.GetProxy().GetAddress()
		if address != "" {
			address = r.scheme + "://" + address
		}
		return address
	}

	slog.DebugContext(ctx, "Topology Service returned non-PROXY response, falling back to default host",
		slog.String("action", resp.GetAction().String()))

	return ""
}

// ResolveByRoute resolves a cell address from a repository path.
// It extracts the top-level namespace and creates a RouteClaim.
// Suitable for repo-scoped endpoints: /allowed, /lfs_authenticate, /git_audit_event.
func (r *Resolver) ResolveByRoute(ctx context.Context, repoPath string) string {
	if r == nil {
		return ""
	}
	namespace := ExtractTopLevelNamespace(repoPath)
	if namespace == "" {
		return ""
	}
	return r.Resolve(ctx, RouteClaim(namespace))
}

// ExtractTopLevelNamespace returns the first path segment from a
// repository path (the top-level namespace in GitLab).
// Examples:
//   - "group/project.git" → "group"
//   - "group" → "group" (single-segment paths are valid for top-level namespaces)
//   - "" → ""
func ExtractTopLevelNamespace(repo string) string {
	repo = strings.TrimLeft(repo, "/")
	repo = strings.TrimSuffix(repo, ".git")
	if i := strings.IndexByte(repo, '/'); i > 0 {
		return repo[:i]
	}
	return repo
}

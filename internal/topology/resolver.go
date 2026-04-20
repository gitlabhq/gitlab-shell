package topology

import (
	"context"
	"strings"

	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	types_proto "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto/types/v1"
	"gitlab.com/gitlab-org/labkit/log"
)

// Resolver queries the Topology Service to determine which cell should
// handle a request. It gracefully degrades: if the TS is disabled,
// unreachable, or returns an error, an empty string is returned and
// the caller should use the default host.
type Resolver struct {
	client *Client
}

// NewResolver creates a new Resolver. If client is nil (TS disabled),
// all Resolve calls return empty string immediately.
func NewResolver(client *Client) *Resolver {
	return &Resolver{client: client}
}

// Resolve queries the Topology Service with the given claim and returns
// the proxy address as an HTTP URL string (e.g. "http://cell-2:8080").
// Returns empty string on any failure or when TS is not configured.
func (r *Resolver) Resolve(ctx context.Context, claim *types_proto.Claim) string {
	if r.client == nil || claim == nil {
		return ""
	}

	resp, err := r.client.Classify(ctx, claim)
	if err != nil {
		log.WithContextFields(ctx, log.Fields{
			"error_message": err.Error(),
		}).Warn("Topology Service classify failed, falling back to default host")
		return ""
	}

	if resp.GetAction() == pb.ClassifyAction_PROXY && resp.GetProxy() != nil {
		address := resp.GetProxy().GetAddress()
		if address != "" && !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
			address = "http://" + address
		}
		return address
	}

	return ""
}

// ResolveByRoute resolves a cell address from a repository path.
// It extracts the top-level namespace and creates a RouteClaim.
// Suitable for repo-scoped endpoints: /allowed, /lfs_authenticate, /git_audit_event.
func (r *Resolver) ResolveByRoute(ctx context.Context, repoPath string) string {
	namespace := ExtractTopLevelNamespace(repoPath)
	if namespace == "" {
		return ""
	}
	return r.Resolve(ctx, RouteClaim(namespace))
}

// ExtractTopLevelNamespace returns the first path segment from a
// repository path (the top-level namespace in GitLab).
// Example: "group/project.git" → "group".
func ExtractTopLevelNamespace(repo string) string {
	repo = strings.TrimPrefix(repo, "/")
	repo = strings.TrimSuffix(repo, ".git")
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return parts[0]
}

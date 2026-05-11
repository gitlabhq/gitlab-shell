package topology

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	types_proto "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto/types/v1"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/labkit/v2/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	classifyMaxAttempts     = 3
	classifyInitialInterval = 50 * time.Millisecond
	classifyMaxInterval     = 250 * time.Millisecond
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
// Transient errors are retried with exponential backoff.
func (r *Resolver) Resolve(ctx context.Context, claim *types_proto.Claim) string {
	if r == nil || r.client == nil || claim == nil {
		return ""
	}

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = classifyInitialInterval
	b.MaxInterval = classifyMaxInterval

	resp, err := backoff.Retry(ctx, func() (*pb.ClassifyResponse, error) {
		resp, err := r.client.Classify(ctx, claim)
		if err != nil && !isRetryableError(err) {
			return resp, backoff.Permanent(err)
		}
		return resp, err
	},
		backoff.WithBackOff(b),
		backoff.WithMaxTries(classifyMaxAttempts),
		backoff.WithNotify(func(err error, duration time.Duration) {
			slog.InfoContext(ctx, "Topology Service classify attempt failed, retrying",
				slog.Duration("retry_in", duration),
				log.ErrorMessage(err.Error()),
			)
		}),
	)
	if err != nil {
		slog.WarnContext(ctx, "Topology Service classify failed after retries, falling back to default host",
			slog.Int("max_attempts", classifyMaxAttempts),
			log.ErrorMessage(err.Error()),
		)
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

// ResolveBySSHKey resolves a cell address from an SSH key identifier.
// The key is an opaque string used as the Topology Service SSHKeyClaim.
// Callers are responsible for choosing a value that the Topology Service
// recognizes; common values are:
//   - the raw "key" string passed by sshd to /authorized_keys (base64 body), and
//   - the SHA256 fingerprint of a signing CA (without the "SHA256:" prefix)
//     for /authorized_certs.
//
// The Topology Service is the source of truth for how these claims are matched.
func (r *Resolver) ResolveBySSHKey(ctx context.Context, key string) string {
	if key == "" {
		return ""
	}
	return r.Resolve(ctx, SSHKeyClaim(key))
}

// ClientForSSHKey returns httpClient routed to the cell resolved for key,
// or the original httpClient if the Topology Service is not configured,
// returns an error, or returns a non-PROXY action.
func (r *Resolver) ClientForSSHKey(ctx context.Context, httpClient *client.GitlabNetClient, key string) *client.GitlabNetClient {
	if host := r.ResolveBySSHKey(ctx, key); host != "" {
		return httpClient.WithHost(host)
	}
	return httpClient
}

// ClientForRoute returns httpClient routed to the cell resolved for repoPath,
// or the original httpClient if the Topology Service is not configured,
// returns an error, or returns a non-PROXY action.
func (r *Resolver) ClientForRoute(ctx context.Context, httpClient *client.GitlabNetClient, repoPath string) *client.GitlabNetClient {
	if host := r.ResolveByRoute(ctx, repoPath); host != "" {
		return httpClient.WithHost(host)
	}
	return httpClient
}

// UserArgs holds the user identity fields needed for cell resolution.
// It mirrors the relevant fields from commandargs.Shell but avoids
// importing the command layer into the topology package.
type UserArgs struct {
	Username      string
	KeyID         string
	Krb5Principal string
}

// ResolveByUserArgs resolves a cell address from the user identity in
// command arguments. It picks the best available claim type:
//   - Username → UsernameClaim
//   - KeyID / Krb5Principal → returns "" (default host fallback, no matching
//     Topology Service claim type)
//
// This is used for user-scoped endpoints (/discover, /two_factor_recovery_codes,
// /two_factor_manual_otp_check, /two_factor_push_otp_check, /personal_access_token)
// that have no repository path for route-based classification.
func (r *Resolver) ResolveByUserArgs(ctx context.Context, args UserArgs) string {
	if r == nil {
		return ""
	}
	if args.Username != "" {
		return r.Resolve(ctx, UsernameClaim(args.Username))
	}
	return ""
}

// ClientForUserArgs returns httpClient routed to the cell resolved for the
// user identity in args, or the original httpClient if the Topology Service
// is not configured, returns an error, or returns a non-PROXY action.
func (r *Resolver) ClientForUserArgs(ctx context.Context, httpClient *client.GitlabNetClient, args UserArgs) *client.GitlabNetClient {
	if host := r.ResolveByUserArgs(ctx, args); host != "" {
		return httpClient.WithHost(host)
	}
	return httpClient
}

// isRetryableError returns true if the gRPC error is transient and the
// request should be retried. Non-gRPC errors are assumed retryable
// (e.g., connection failures).
func isRetryableError(err error) bool {
	s, ok := status.FromError(err)
	if !ok {
		// Not a gRPC status error (e.g., connection error) — retry.
		return true
	}

	switch s.Code() {
	case codes.Unavailable, codes.ResourceExhausted, codes.Aborted,
		codes.Internal, codes.DeadlineExceeded, codes.Unknown:
		return true
	default:
		return false
	}
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

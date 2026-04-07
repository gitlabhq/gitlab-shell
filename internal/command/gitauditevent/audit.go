// Package gitauditevent handles Git audit events for GitLab.
package gitauditevent

import (
	"context"
	"fmt"
	"log/slog"

	pb "gitlab.com/gitlab-org/gitaly/v18/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/gitauditevent"
	"gitlab.com/gitlab-org/labkit/v2/log"
)

// Audit is called conditionally during `git-receive-pack` and `git-upload-pack` to generate streaming audit events.
// Errors are not propagated since this is more a logging process.
func Audit(ctx context.Context, args *commandargs.Shell, c *config.Config, response *accessverifier.Response, packfileStats *pb.PackfileNegotiationStatistics) {
	ctx = log.WithFields(ctx,
		slog.String("gl_repository", response.Repo),
		slog.Any("command", args.CommandType),
		log.GitLabUserName(response.Username),
		slog.String("gl_key_type", response.KeyType),
		slog.Int("gl_key_id", response.KeyID),
	)

	slog.DebugContext(ctx, "sending git audit event")

	gitAuditClient, errOnlyLog := gitauditevent.NewClient(c)
	if errOnlyLog != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("failed to create gitauditevent client: %v", errOnlyLog))
		return
	}

	errOnlyLog = gitAuditClient.Audit(ctx, gitauditevent.AuditParams{
		Username:      response.Username,
		KeyID:         response.KeyID,
		Repo:          response.Repo,
		PackfileStats: packfileStats,
	}, args)
	if errOnlyLog != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("failed to audit git event: %v", errOnlyLog))
		return
	}
}

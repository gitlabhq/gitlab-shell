package gitauditevent

import (
	"context"

	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/gitauditevent"
	"gitlab.com/gitlab-org/labkit/log"
)

// Audit is a command function that call rails to audit `git-receive-pack` and `git-upload-pack` events.
// Calling rails for git audit events is always followed by `git clone`, `git pull`, `git push`,
// So we do not guarantee that two requests will succeed, this is a Distributed transaction.
// Let's just simply ignore errors.
func Audit(ctx context.Context, commandType commandargs.CommandType, c *config.Config, response *accessverifier.Response, packfileStats *pb.PackfileNegotiationStatistics) {
	ctxlog := log.WithContextFields(ctx, log.Fields{
		"gl_repository": response.Repo,
		"command":       commandType,
		"username":      response.Username,
	})

	ctxlog.Debug("sending git audit event")

	gitAuditClient, errOnlyLog := gitauditevent.NewClient(c)
	if errOnlyLog != nil {
		ctxlog.Errorf("failed to create gitauditevent client: %v", errOnlyLog)
		return
	}

	errOnlyLog = gitAuditClient.Audit(ctx, response.Username, commandType, response.Repo, packfileStats)
	if errOnlyLog != nil {
		ctxlog.Errorf("failed to audit git event: %v", errOnlyLog)
		return
	}
}

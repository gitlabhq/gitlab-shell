// Package uploadarchive provides functionality for uploading archives
package uploadarchive

import (
	"context"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// Command represents the upload archive command
type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

type logInfo struct{}

// Execute executes the upload archive command
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	args := c.Args.SshArgs
	if len(args) != 2 {
		return ctx, disallowedcommand.Error
	}

	repo := args[1]
	response, err := c.verifyAccess(ctx, repo)
	if err != nil {
		return ctx, err
	}

	logData := command.NewLogData(
		response.Gitaly.Repo.GlProjectPath,
		response.Username,
		response.ProjectID,
		response.RootNamespaceID,
	)
	ctxWithLogData := context.WithValue(ctx, logInfo{}, logData)

	return ctxWithLogData, c.performGitalyCall(ctx, response)
}

func (c *Command) verifyAccess(ctx context.Context, repo string) (*accessverifier.Response, error) {
	cmd := accessverifier.Command{
		Config:     c.Config,
		Args:       c.Args,
		ReadWriter: c.ReadWriter,
	}

	return cmd.Verify(ctx, c.Args.CommandType, repo)
}

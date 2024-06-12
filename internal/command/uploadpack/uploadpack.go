// Package uploadpack provides functionality for handling upload-pack command.
package uploadpack

import (
	"context"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/githttp"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/gitauditevent"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/customaction"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// Command represents the upload-pack command
type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

type logDataKey struct{}

// Execute executes the upload-pack command
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
	ctxWithLogData := context.WithValue(ctx, logDataKey{}, logData)

	if response.IsCustomAction() {
		if response.Payload.Data.GeoProxyFetchDirectToPrimary {
			cmd := githttp.PullCommand{
				Config:     c.Config,
				ReadWriter: c.ReadWriter,
				Args:       c.Args,
				Response:   response,
			}

			return ctxWithLogData, cmd.Execute(ctx)
		}

		customAction := customaction.Command{
			Config:     c.Config,
			ReadWriter: c.ReadWriter,
			EOFSent:    false,
		}
		return ctxWithLogData, customAction.Execute(ctx, response)
	}

	stats, err := c.performGitalyCall(ctx, response)
	if err != nil {
		return ctxWithLogData, err
	}

	if response.NeedAudit {
		gitauditevent.Audit(ctx, c.Args.CommandType, c.Config, response, stats)
	}
	return ctxWithLogData, nil
}

func (c *Command) verifyAccess(ctx context.Context, repo string) (*accessverifier.Response, error) {
	cmd := accessverifier.Command{
		Config:     c.Config,
		Args:       c.Args,
		ReadWriter: c.ReadWriter,
	}

	return cmd.Verify(ctx, c.Args.CommandType, repo)
}

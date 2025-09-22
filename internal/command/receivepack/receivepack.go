// Package receivepack provides functionality for handling Git receive-pack commands
package receivepack

import (
	"context"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/gitauditevent"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/githttp"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

// Command represents the receive-pack command
type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

type logData struct{}

// Execute executes the receive-pack command
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	args := c.Args.SSHArgs
	if len(args) != 2 {
		return ctx, disallowedcommand.Error
	}

	repo := args[1]
	response, err := c.verifyAccess(ctx, repo)
	if err != nil {
		return ctx, err
	}

	ctxWithLogData := context.WithValue(ctx, logData{}, command.NewLogData(
		response.Gitaly.Repo.GlProjectPath,
		response.Username,
		response.ProjectID,
		response.RootNamespaceID,
	))

	if response.IsCustomAction() {
		// A Git over HTTP direct request to primary repo is performed
		// (instead of formerly proxying the request through Gitlab Rails).
		cmd := githttp.PushCommand{
			Config:     c.Config,
			ReadWriter: c.ReadWriter,
			Response:   response,
			Args:       c.Args,
		}

		return ctxWithLogData, cmd.Execute(ctx)
	}

	err = c.performGitalyCall(ctx, response)
	if err != nil {
		return ctxWithLogData, err
	}

	if response.NeedAudit {
		gitauditevent.Audit(ctx, c.Args, c.Config, response, nil /* keep nil for `git-receive-pack`*/)
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

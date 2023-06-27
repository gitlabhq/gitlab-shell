package uploadpack

import (
	"context"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/customaction"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) (*accessverifier.Response, error) {
	args := c.Args.SshArgs
	if len(args) != 2 {
		return nil, disallowedcommand.Error
	}

	repo := args[1]
	response, err := c.verifyAccess(ctx, repo)
	if err != nil {
		return nil, err
	}

	if response.IsCustomAction() {
		customAction := customaction.Command{
			Config:     c.Config,
			ReadWriter: c.ReadWriter,
			EOFSent:    false,
		}
		return response, customAction.Execute(ctx, response)
	}

	return response, c.performGitalyCall(ctx, response)
}

func (c *Command) verifyAccess(ctx context.Context, repo string) (*accessverifier.Response, error) {
	cmd := accessverifier.Command{c.Config, c.Args, c.ReadWriter}

	return cmd.Verify(ctx, c.Args.CommandType, repo)
}

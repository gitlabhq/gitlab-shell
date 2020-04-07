package receivepack

import (
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/customaction"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute() error {
	args := c.Args.SshArgs
	if len(args) != 2 {
		return disallowedcommand.Error
	}

	repo := args[1]
	response, err := c.verifyAccess(repo)
	if err != nil {
		return err
	}

	if response.IsCustomAction() {
		customAction := customaction.Command{c.Config, c.ReadWriter}
		return customAction.Execute(response)
	}

	return c.performGitalyCall(response)
}

func (c *Command) verifyAccess(repo string) (*accessverifier.Response, error) {
	cmd := accessverifier.Command{c.Config, c.Args, c.ReadWriter}

	return cmd.Verify(c.Args.CommandType, repo)
}

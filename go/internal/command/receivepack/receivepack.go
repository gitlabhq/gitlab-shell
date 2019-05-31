package receivepack

import (
	"errors"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

var (
	disallowedCommandError = errors.New("> GitLab: Disallowed command")
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.CommandArgs
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute() error {
	args := c.Args.SshArgs
	if len(args) != 2 {
		return disallowedCommandError
	}

	repo := args[1]
	response, err := c.verifyAccess(repo)
	if err != nil {
		return err
	}

	if response.IsCustomAction() {
		return c.processCustomAction(response)
	}

	return c.performGitalyCall(response)
}

func (c *Command) verifyAccess(repo string) (*accessverifier.Response, error) {
	cmd := accessverifier.Command{c.Config, c.Args, c.ReadWriter}

	return cmd.Verify(c.Args.CommandType, repo)
}

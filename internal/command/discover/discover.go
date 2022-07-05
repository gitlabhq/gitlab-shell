package discover

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/discover"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) error {
	response, err := c.getUserInfo(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get username: %v", err)
	}

	if response.IsAnonymous() {
		fmt.Fprintf(c.ReadWriter.Out, "Welcome to GitLab, Anonymous!\n")
	} else {
		fmt.Fprintf(c.ReadWriter.Out, "Welcome to GitLab, @%s!\n", response.Username)
	}

	return nil
}

func (c *Command) getUserInfo(ctx context.Context) (*discover.Response, error) {
	client, err := discover.NewClient(c.Config)
	if err != nil {
		return nil, err
	}

	return client.GetByCommandArgs(ctx, c.Args)
}

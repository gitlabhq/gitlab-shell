package authorizedkeys

import (
	"context"
	"fmt"
	"strconv"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/authorizedkeys"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/keyline"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.AuthorizedKeys
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) error {
	// Do and return nothing when the expected and actual user don't match.
	// This can happen when the user in sshd_config doesn't match the user
	// trying to login. When nothing is printed, the user will be denied access.
	if c.Args.ExpectedUser != c.Args.ActualUser {
		// TODO: Log this event once we have a consistent way to log in Go.
		// See https://gitlab.com/gitlab-org/gitlab-shell/issues/192 for more info.
		return nil
	}

	if err := c.printKeyLine(ctx); err != nil {
		return err
	}

	return nil
}

func (c *Command) printKeyLine(ctx context.Context) error {
	response, err := c.getAuthorizedKey(ctx)
	if err != nil {
		fmt.Fprintln(c.ReadWriter.Out, fmt.Sprintf("# No key was found for %s", c.Args.Key))
		return nil
	}

	keyLine, err := keyline.NewPublicKeyLine(strconv.FormatInt(response.Id, 10), response.Key, c.Config)
	if err != nil {
		return err
	}

	fmt.Fprintln(c.ReadWriter.Out, keyLine.ToString())

	return nil
}

func (c *Command) getAuthorizedKey(ctx context.Context) (*authorizedkeys.Response, error) {
	client, err := authorizedkeys.NewClient(c.Config)
	if err != nil {
		return nil, err
	}

	return client.GetByKey(ctx, c.Args.Key)
}

// Package authorizedkeys handles fetching and printing authorized SSH keys.
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

// Command contains the configuration, arguments, and I/O interfaces.
type Command struct {
	Config     *config.Config
	Args       *commandargs.AuthorizedKeys
	ReadWriter *readwriter.ReadWriter
}

// Execute runs the command to fetch and print the authorized SSH key.
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	// Do and return nothing when the expected and actual user don't match.
	// This can happen when the user in sshd_config doesn't match the user
	// trying to login. When nothing is printed, the user will be denied access.
	if c.Args.ExpectedUser != c.Args.ActualUser {
		return ctx, nil
	}

	if err := c.printKeyLine(ctx); err != nil {
		return ctx, err
	}

	return ctx, nil
}

func (c *Command) printKeyLine(ctx context.Context) error {
	response, err := c.getAuthorizedKey(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(c.ReadWriter.Out, "# No key was found for %s\n", c.Args.Key)
		return nil
	}

	keyLine, err := keyline.NewPublicKeyLine(strconv.FormatInt(response.ID, 10), response.Key, c.Config)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(c.ReadWriter.Out, keyLine.ToString())

	return nil
}

func (c *Command) getAuthorizedKey(ctx context.Context) (*authorizedkeys.Response, error) {
	client, err := authorizedkeys.NewClient(c.Config)
	if err != nil {
		return nil, err
	}

	return client.GetByKey(ctx, c.Args.Key)
}

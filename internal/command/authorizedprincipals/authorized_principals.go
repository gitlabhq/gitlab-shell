// Package authorizedprincipals handles printing authorized principals in GitLab Shell.
package authorizedprincipals

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/keyline"
)

// Command contains the configuration, arguments, and I/O interfaces.
type Command struct {
	Config     *config.Config
	Args       *commandargs.AuthorizedPrincipals
	ReadWriter *readwriter.ReadWriter
}

// Execute runs the command to print authorized principals.
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	if err := c.printPrincipalLines(); err != nil {
		return ctx, err
	}

	return ctx, nil
}

func (c *Command) printPrincipalLines() error {
	principals := c.Args.Principals

	for _, principal := range principals {
		if err := c.printPrincipalLine(principal); err != nil {
			return err
		}
	}

	return nil
}

func (c *Command) printPrincipalLine(principal string) error {
	principalKeyLine, err := keyline.NewPrincipalKeyLine(c.Args.KeyID, principal, c.Config)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(c.ReadWriter.Out, principalKeyLine.ToString())

	return nil
}

package authorizedprincipals

import (
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/keyline"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.AuthorizedPrincipals
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute() error {
	if err := c.printPrincipalLines(); err != nil {
		return err
	}

	return nil
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
	principalKeyLine, err := keyline.NewPrincipalKeyLine(c.Args.KeyId, principal, c.Config.RootDir)
	if err != nil {
		return err
	}

	fmt.Fprintln(c.ReadWriter.Out, principalKeyLine.ToString())

	return nil
}

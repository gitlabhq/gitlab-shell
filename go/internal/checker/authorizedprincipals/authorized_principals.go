package authorizedprincipals

import (
	"errors"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/checker/keyline"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

type Checker struct {
	Config     *config.Config
	Args       []string
	ReadWriter *readwriter.ReadWriter
}

func (c *Checker) Execute() error {
	if err := c.validateArguments(); err != nil {
		return err
	}

	args := c.Args
	keyId := args[0]
	principals := args[1:]

	if err := c.printPrincipalLines(keyId, principals); err != nil {
		return err
	}

	return nil
}

func (c *Checker) validateArguments() error {
	args := c.Args
	argsSize := len(args)

	if argsSize < 2 {
		return errors.New(fmt.Sprintf("# Wrong number of arguments. %d. Usage\n#\tgitlab-shell-authorized-principals-check <key-id> <principal1> [<principal2>...]", argsSize))
	}

	keyId := args[0]
	principals := args[1:]

	if keyId == "" {
		return errors.New("# No key_id provided")
	}

	for _, principal := range principals {
		if principal == "" {
			return errors.New("# An invalid principal was provided")
		}
	}

	return nil
}

func (c *Checker) printPrincipalLines(keyId string, principals []string) error {
	for _, principal := range principals {
		principalKeyLine := &keyline.KeyLine{
			Id:      keyId,
			Value:   principal,
			Prefix:  "username",
			RootDir: c.Config.RootDir,
		}

		line, err := principalKeyLine.ToString()
		if err != nil {
			return err
		}

		fmt.Fprintln(c.ReadWriter.Out, line)
	}

	return nil
}

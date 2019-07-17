package authorizedprincipals

import (
	"errors"
	"fmt"
	"path"

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

	c.printPrincipalLines(keyId, principals)

	return nil
}

func (c *Checker) printPrincipalLines(keyId string, principals []string) {
	command := fmt.Sprintf("%s username-%s", path.Join(c.Config.RootDir, "bin", "gitlab-shell"), keyId)

	for _, principal := range principals {
		principalLine := fmt.Sprintf(`command="%s",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty %s`, command, principal)

		fmt.Fprintln(c.ReadWriter.Out, principalLine)
	}
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

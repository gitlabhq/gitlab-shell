package authorizedkeys

import (
	"errors"
	"fmt"
	"path"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/authorizedkeys"
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
	expectedUsername := args[0]
	actualUsername := args[1]
	key := args[2]

	if expectedUsername == actualUsername {
		if err := c.printKeyLine(key); err != nil {
			return fmt.Errorf("Failed to print key line: %v", err)
		}
	}

	return nil
}

func (c *Checker) printKeyLine(key string) error {
	client, err := authorizedkeys.NewClient(c.Config)
	if err != nil {
		return err
	}

	response, err := client.GetByKey(key)
	if err != nil {
		fmt.Fprintln(c.ReadWriter.Out, fmt.Sprintf("# No key was found for %s", key))
	} else {
		fmt.Fprintln(c.ReadWriter.Out, c.formatKeyLine(response.Id, response.Key))
	}

	return nil
}

func (c *Checker) formatKeyLine(id int64, key string) string {
	command := fmt.Sprintf("%s key-%d", path.Join(c.Config.RootDir, "bin", "gitlab-shell"), id)

	return fmt.Sprintf(`command="%s",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty %s`, command, key)
}

func (c *Checker) validateArguments() error {
	args := c.Args
	argsSize := len(args)

	if argsSize != 3 {
		return errors.New(fmt.Sprintf("# Wrong number of arguments. %d. Usage\n#\tgitlab-shell-authorized-keys-check <expected-username> <actual-username> <key>", argsSize))
	}

	expectedUsername := args[0]
	actualUsername := args[1]
	key := args[2]

	if expectedUsername == "" || actualUsername == "" {
		return errors.New("# No username provided")
	}

	if key == "" {
		return errors.New("# No key provided")
	}

	return nil
}

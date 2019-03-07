package discover

import (
	"fmt"
	"io"
	"os"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/discover"
)

type Command struct {
	Config *config.Config
	Args   *commandargs.CommandArgs
}

var (
	output io.Writer = os.Stdout
)

func (c *Command) Execute() error {
	response, err := c.getUserInfo()
	if err != nil {
		return fmt.Errorf("Failed to get username: %v", err)
	}

	if response.IsAnonymous() {
		fmt.Fprintf(output, "Welcome to GitLab, Anonymous!\n")
	} else {
		fmt.Fprintf(output, "Welcome to GitLab, @%s!\n", response.Username)
	}

	return nil
}

func (c *Command) getUserInfo() (*discover.Response, error) {
	client, err := discover.NewClient(c.Config)
	if err != nil {
		return nil, err
	}

	if c.Args.GitlabKeyId != "" {
		return client.GetByKeyId(c.Args.GitlabKeyId)
	} else if c.Args.GitlabUsername != "" {
		return client.GetByUsername(c.Args.GitlabUsername)
	} else {
		// There was no 'who' information, this  matches the ruby error
		// message.
		return nil, fmt.Errorf("who='' is invalid")
	}
}

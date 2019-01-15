package discover

import (
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

type Command struct {
	Config *config.Config
	Args   *commandargs.CommandArgs
}

func (c *Command) Execute() error {
	return fmt.Errorf("No feature is implemented yet")
}

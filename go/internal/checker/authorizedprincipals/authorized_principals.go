package authorizedprincipals

import (
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
)

type Checker struct {
	Config     *config.Config
	Args       []string
	ReadWriter *readwriter.ReadWriter
}

func (c *Checker) Execute() error {
	return nil
}

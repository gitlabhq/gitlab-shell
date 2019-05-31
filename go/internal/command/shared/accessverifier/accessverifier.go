package accessverifier

import (
	"errors"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/accessverifier"
)

type Response = accessverifier.Response

type Command struct {
	Config     *config.Config
	Args       *commandargs.CommandArgs
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Verify(action commandargs.CommandType, repo string) (*Response, error) {
	client, err := accessverifier.NewClient(c.Config)
	if err != nil {
		return nil, err
	}

	response, err := client.Verify(c.Args, action, repo)
	if err != nil {
		return nil, err
	}

	c.displayConsoleMessages(response.ConsoleMessages)

	if !response.Success {
		return nil, errors.New(response.Message)
	}

	return response, nil
}

func (c *Command) displayConsoleMessages(messages []string) {
	for _, msg := range messages {
		fmt.Fprintf(c.ReadWriter.ErrOut, "> GitLab: %v\n", msg)
	}
}

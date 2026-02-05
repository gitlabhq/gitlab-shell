// Package accessverifier handles the verification of access permission.
package accessverifier

import (
	"context"
	"errors"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/console"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
)

// Response is an alias for accessverifier.Response, representing the result of an access verification.
type Response = accessverifier.Response

// Command handles access verification commands.
type Command struct {
	GitlabClient *client.GitlabNetClient
	Args         *commandargs.Shell
	ReadWriter   *readwriter.ReadWriter
}

// Verify checks access permissions and returns a response.
func (c *Command) Verify(ctx context.Context, action commandargs.CommandType, repo string) (*Response, error) {
	client, err := accessverifier.NewClient(c.GitlabClient)
	if err != nil {
		return nil, err
	}

	response, err := client.Verify(ctx, c.Args, action, repo)
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
	console.DisplayInfoMessages(messages, c.ReadWriter.ErrOut)
}

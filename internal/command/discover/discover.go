// Package discover implements the "discover" command for fetching user info and displaying a welcome message.
package discover

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/discover"
)

type logDataKey struct{}

// Command struct encapsulates the necessary components for executing the Discover command.
type Command struct {
	Args         *commandargs.Shell
	ReadWriter   *readwriter.ReadWriter
	GitlabClient *client.GitlabNetClient
}

// Execute runs the discover command, fetching and displaying user information.
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	response, err := c.getUserInfo(ctx)
	if err != nil {
		return ctx, fmt.Errorf("Failed to get username: %v", err) //nolint:stylecheck // This is customer facing message
	}

	logData := command.LogData{}
	if response.IsAnonymous() {
		logData.Username = "Anonymous"
		_, _ = fmt.Fprintf(c.ReadWriter.Out, "Welcome to GitLab, Anonymous!\n")
	} else {
		logData.Username = response.Username
		_, _ = fmt.Fprintf(c.ReadWriter.Out, "Welcome to GitLab, @%s!\n", response.Username)
	}

	ctxWithLogData := context.WithValue(ctx, logDataKey{}, logData)

	return ctxWithLogData, nil
}

func (c *Command) getUserInfo(ctx context.Context) (*discover.Response, error) {
	client, err := discover.NewClient(c.GitlabClient)
	if err != nil {
		return nil, err
	}

	return client.GetByCommandArgs(ctx, c.Args)
}

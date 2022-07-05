package healthcheck

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/healthcheck"
)

var (
	apiMessage   = "Internal API available"
	redisMessage = "Redis available via internal API"
)

type Command struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) error {
	response, err := c.runCheck(ctx)
	if err != nil {
		return fmt.Errorf("%v: FAILED - %v", apiMessage, err)
	}

	fmt.Fprintf(c.ReadWriter.Out, "%v: OK\n", apiMessage)

	if !response.Redis {
		return fmt.Errorf("%v: FAILED", redisMessage)
	}

	fmt.Fprintf(c.ReadWriter.Out, "%v: OK\n", redisMessage)
	return nil
}

func (c *Command) runCheck(ctx context.Context) (*healthcheck.Response, error) {
	client, err := healthcheck.NewClient(c.Config)
	if err != nil {
		return nil, err
	}

	response, err := client.Check(ctx)
	if err != nil {
		return nil, err
	}

	return response, nil
}

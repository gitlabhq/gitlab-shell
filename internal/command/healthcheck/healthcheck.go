package healthcheck

import (
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/healthcheck"
)

var (
	apiMessage   = "Internal API available"
	redisMessage = "Redis available via internal API"
)

type Command struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute() error {
	response, err := c.runCheck()
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

func (c *Command) runCheck() (*healthcheck.Response, error) {
	client, err := healthcheck.NewClient(c.Config)
	if err != nil {
		return nil, err
	}

	response, err := client.Check()
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Package healthcheck provides functionality to perform health checks.
package healthcheck

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

var (
	apiMessage   = "Internal API available"
	redisMessage = "Redis available via internal API"
)

// Command handles the execution of health checks.
type Command struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
}

// Execute performs the health check and outputs the result.
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	healthCheck, err := c.runCheck(ctx)
	if err != nil {
		return ctx, fmt.Errorf("%v: FAILED - %v", apiMessage, err)
	}

	_, _ = fmt.Fprintf(c.ReadWriter.Out, "%v: OK\n", apiMessage)

	if !healthCheck.IsRedisHealthy {
		return ctx, fmt.Errorf("%v: FAILED", redisMessage)
	}

	_, _ = fmt.Fprintf(c.ReadWriter.Out, "%v: OK\n", redisMessage)
	return ctx, nil
}

type checkResponse struct {
	IsRedisHealthy bool
}

func (c *Command) runCheck(ctx context.Context) (*checkResponse, error) {
	client, err := client.New(client.ClientOpts{
		GitlabURL:             c.Config.GitlabURL,
		GitlabRelativeURLRoot: c.Config.GitlabRelativeURLRoot,
		CAFile:                c.Config.HTTPSettings.CaFile,
		CAPath:                c.Config.HTTPSettings.CaPath,
		ReadTimeoutSeconds:    c.Config.HTTPSettings.ReadTimeoutSeconds,
		User:                  c.Config.HTTPSettings.User,
		Password:              c.Config.HTTPSettings.Password,
		Secret:                c.Config.Secret,
	})
	if err != nil {
		return nil, err
	}

	response, err := client.CheckHealth(ctx)
	if err != nil {
		return nil, err
	}

	return &checkResponse{
		IsRedisHealthy: response.Redis,
	}, nil
}

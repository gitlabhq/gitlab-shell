// Package healthcheck provides functionality to perform health checks.
package healthcheck

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
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
	response, err := c.runCheck(ctx)
	if err != nil {
		return ctx, fmt.Errorf("%v: FAILED - %v", apiMessage, err)
	}

	_, _ = fmt.Fprintf(c.ReadWriter.Out, "%v: OK\n", apiMessage)

	if !response.Redis {
		return ctx, fmt.Errorf("%v: FAILED", redisMessage)
	}

	_, _ = fmt.Fprintf(c.ReadWriter.Out, "%v: OK\n", redisMessage)
	return ctx, nil
}

// runCheck queries the internal API health endpoint using the unified gitlab
// client. The legacy gitlabnet healthcheck client and its feature-flag gate
// were removed once acceptance tests confirmed the two paths were equivalent.
func (c *Command) runCheck(ctx context.Context) (*gitlab.HealthcheckResponse, error) {
	newClient, err := gitlab.New(&gitlab.Config{
		GitlabURL:          c.Config.GitlabURL,
		User:               c.Config.HTTPSettings.User,
		Password:           c.Config.HTTPSettings.Password,
		Secret:             c.Config.Secret,
		CaFile:             c.Config.HTTPSettings.CaFile,
		CaPath:             c.Config.HTTPSettings.CaPath,
		ReadTimeoutSeconds: c.Config.HTTPSettings.ReadTimeoutSeconds,
	})
	if err != nil {
		return nil, err
	}

	return gitlab.NewHealthcheckClient(newClient).Check(ctx)
}

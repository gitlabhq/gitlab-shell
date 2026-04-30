// Package healthcheck provides functionality to perform health checks.
package healthcheck

import (
	"context"
	"fmt"

	"github.com/open-feature/go-sdk/openfeature"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/healthcheck"
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

func (c *Command) runCheck(ctx context.Context) (*healthcheck.Response, error) {
	// Check if we should use the new client via feature flag
	evaluator := command.FeatureFlagEvaluatorFromContext(ctx)
	if evaluator != nil {
		// Use a stable targeting key so the Flipt provider doesn't bail
		// out with TargetingKeyMissingResolutionError before making the
		// HTTP call. Healthcheck is a per-process operation with no user
		// identity, so a constant key is the right grain — flag rules
		// keyed on this should target the gitlab-shell service rather
		// than individual entities.
		evalCtx := openfeature.NewEvaluationContext("healthcheck", nil)
		details, err := evaluator.BooleanValueDetails(ctx, "use_new_healthcheck_client", false, evalCtx)
		if err == nil && details.Value {
			return c.runCheckNewClient(ctx)
		}
		// If flag check fails or flag is false, fall through to old client
	}

	// Use the old client (default, safe path)
	return c.runCheckOldClient(ctx)
}

// runCheckOldClient uses the old gitlabnet healthcheck client.
func (c *Command) runCheckOldClient(ctx context.Context) (*healthcheck.Response, error) {
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

// runCheckNewClient uses the new internal/clients/gitlab healthcheck client.
func (c *Command) runCheckNewClient(ctx context.Context) (*healthcheck.Response, error) {
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

	hc := gitlab.NewHealthcheckClient(newClient)
	resp, err := hc.Check(ctx)
	if err != nil {
		return nil, err
	}

	// Convert from gitlab.HealthcheckResponse to healthcheck.Response
	return &healthcheck.Response{
		APIVersion:     resp.APIVersion,
		GitlabVersion:  resp.GitlabVersion,
		GitlabRevision: resp.GitlabRevision,
		Redis:          resp.Redis,
	}, nil
}

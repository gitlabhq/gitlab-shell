package receivepack

import (
	"context"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/githttp"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/customaction"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) (*accessverifier.Response, error) {
	args := c.Args.SshArgs
	if len(args) != 2 {
		return nil, disallowedcommand.Error
	}

	repo := args[1]
	response, err := c.verifyAccess(ctx, repo)
	if err != nil {
		return nil, err
	}

	if response.IsCustomAction() {
		// When `geo_proxy_direct_to_primary` feature flag is enabled, a Git over HTTP direct request
		// to primary repo is performed instead of proxying the request through Gitlab Rails.
		// After the feature flag is enabled by default and removed,
		// custom action functionality will be removed along with it.
		if response.Payload.Data.GeoProxyDirectToPrimary {
			cmd := githttp.PushCommand{
				Config:     c.Config,
				ReadWriter: c.ReadWriter,
				Response:   response,
			}

			return response, cmd.Execute(ctx)
		}

		customAction := customaction.Command{
			Config:     c.Config,
			ReadWriter: c.ReadWriter,
			EOFSent:    true,
		}
		return response, customAction.Execute(ctx, response)
	}

	return response, c.performGitalyCall(ctx, response)
}

func (c *Command) verifyAccess(ctx context.Context, repo string) (*accessverifier.Response, error) {
	cmd := accessverifier.Command{c.Config, c.Args, c.ReadWriter}

	return cmd.Verify(ctx, c.Args.CommandType, repo)
}

package githttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
)

// CellsPullCommand handles git pull via SSH-over-HTTP for Cells routing.
// When the Topology Service routes to a different Cell, Gitaly is not
// directly reachable. Instead, we proxy the SSH pack data through the
// Cell's Workhorse via POST /{repo}.git/ssh-upload-pack.
type CellsPullCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	Args       *commandargs.Shell
	Response   *accessverifier.Response
}

// Execute runs the Cells SSH-over-HTTP upload-pack operation.
func (c *CellsPullCommand) Execute(ctx context.Context) error {
	slog.InfoContext(ctx, "Cells: using SSH-over-HTTP upload-pack",
		slog.String("cell_address", c.Response.CellAddress))

	gitClient, err := buildCellsGitClient(c.Config, c.Response, c.Args)
	if err != nil {
		return err
	}
	return executeSSHRequest(ctx, gitClient.SSHUploadPack, c.ReadWriter)
}

// CellsPushCommand handles git push via SSH-over-HTTP for Cells routing.
type CellsPushCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	Args       *commandargs.Shell
	Response   *accessverifier.Response
}

// Execute runs the Cells SSH-over-HTTP receive-pack operation.
func (c *CellsPushCommand) Execute(ctx context.Context) error {
	slog.InfoContext(ctx, "Cells: using SSH-over-HTTP receive-pack",
		slog.String("cell_address", c.Response.CellAddress))

	gitClient, err := buildCellsGitClient(c.Config, c.Response, c.Args)
	if err != nil {
		return err
	}
	return executeSSHRequest(ctx, gitClient.SSHReceivePack, c.ReadWriter)
}

func buildCellsGitClient(
	cfg *config.Config,
	response *accessverifier.Response,
	args *commandargs.Shell,
) (*git.Client, error) {
	repoPath := response.Gitaly.Repo.GetGlProjectPath()
	if repoPath == "" {
		return nil, fmt.Errorf("cells routing: missing gl_project_path in /allowed response")
	}

	repoURL := fmt.Sprintf("%s/%s.git",
		strings.TrimSuffix(response.CellAddress, "/"), repoPath)

	// Validate that the constructed URL has a scheme. CellAddress should
	// always have one (the Topology Service resolver prepends it), but
	// catch misconfigurations early with a clear error.
	if u, err := url.Parse(repoURL); err != nil || u.Scheme == "" {
		return nil, fmt.Errorf("cells routing: invalid cell address %q: missing URL scheme", response.CellAddress)
	}

	shellJWT, err := client.SignShellJWT(cfg.Secret, response.UserID)
	if err != nil {
		return nil, fmt.Errorf("cells routing: generating Shell JWT: %w", err)
	}

	headers := map[string]string{
		"Gitlab-Shell-Api-Request": shellJWT,
	}

	if args.Env.GitProtocolVersion != "" {
		headers["Git-Protocol"] = args.Env.GitProtocolVersion
	}

	return &git.Client{URL: repoURL, Headers: headers}, nil
}

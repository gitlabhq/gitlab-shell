package githttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
)

// CellsPullCommand handles git pull (upload-pack) via SSH-over-HTTP for Cells
// routing. When the Topology Service routes to a different Cell, Gitaly is not
// directly reachable, so it proxies SSH pack data through the Cell's Workhorse
// via POST /{repo}.git/ssh-upload-pack.
type CellsPullCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	Args       *commandargs.Shell
	Response   *accessverifier.Response
}

// Execute runs a Cells SSH-over-HTTP upload-pack request.
func (c *CellsPullCommand) Execute(ctx context.Context) error {
	slog.InfoContext(ctx, "Cells: using SSH-over-HTTP upload-pack",
		slog.String("cell_address", c.Response.CellAddress))

	gitClient, err := buildCellsGitClient(c.Config, c.Response, c.Args)
	if err != nil {
		return err
	}

	return pipeRequest(ctx, c.ReadWriter, readUploadPackRequest, gitClient.SSHUploadPack)
}

// NewCellsPullCommand builds a Cells SSH-over-HTTP upload-pack command.
func NewCellsPullCommand(cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) *CellsPullCommand {
	return &CellsPullCommand{
		Config:     cfg,
		ReadWriter: rw,
		Args:       args,
		Response:   resp,
	}
}

// CellsPushCommand handles git push (receive-pack) via SSH-over-HTTP for Cells
// routing. When the Topology Service routes to a different Cell, Gitaly is not
// directly reachable, so it proxies SSH pack data through the Cell's Workhorse
// via POST /{repo}.git/ssh-receive-pack.
type CellsPushCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	Args       *commandargs.Shell
	Response   *accessverifier.Response
}

// Execute runs a Cells SSH-over-HTTP receive-pack request.
func (c *CellsPushCommand) Execute(ctx context.Context) error {
	slog.InfoContext(ctx, "Cells: using SSH-over-HTTP receive-pack",
		slog.String("cell_address", c.Response.CellAddress))

	gitClient, err := buildCellsGitClient(c.Config, c.Response, c.Args)
	if err != nil {
		return err
	}

	return executeSSHRequest(ctx, gitClient.SSHReceivePack, c.ReadWriter)
}

// NewCellsPushCommand builds a Cells SSH-over-HTTP receive-pack command.
func NewCellsPushCommand(cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) *CellsPushCommand {
	return &CellsPushCommand{
		Config:     cfg,
		ReadWriter: rw,
		Args:       args,
		Response:   resp,
	}
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

	base, err := url.Parse(response.CellAddress)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("cells routing: invalid cell address %q: missing URL scheme", response.CellAddress)
	}
	base.Path = path.Join(base.Path, repoPath+".git")
	repoURL := base.String()

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

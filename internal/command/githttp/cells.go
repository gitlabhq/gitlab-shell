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

// CellsCommand handles git pull/push via SSH-over-HTTP for Cells routing.
// When the Topology Service routes to a different Cell, Gitaly is not
// directly reachable, so we proxy SSH pack data through the Cell's
// Workhorse via POST /{repo}.git/ssh-upload-pack or /ssh-receive-pack.
type CellsCommand struct {
	Config     *config.Config
	ReadWriter *readwriter.ReadWriter
	Args       *commandargs.Shell
	Response   *accessverifier.Response

	Operation string
	RequestFn func(client *git.Client) sshRequestFunc
}

// Execute runs the Cells SSH-over-HTTP operation.
func (c *CellsCommand) Execute(ctx context.Context) error {
	slog.InfoContext(ctx, "Cells: using SSH-over-HTTP "+c.Operation,
		slog.String("cell_address", c.Response.CellAddress))

	gitClient, err := buildCellsGitClient(c.Config, c.Response, c.Args)
	if err != nil {
		return err
	}
	return executeSSHRequest(ctx, c.RequestFn(gitClient), c.ReadWriter)
}

// NewCellsPullCommand builds a Cells SSH-over-HTTP upload-pack command.
func NewCellsPullCommand(cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) *CellsCommand {
	return &CellsCommand{
		Config:     cfg,
		ReadWriter: rw,
		Args:       args,
		Response:   resp,
		Operation:  "upload-pack",
		RequestFn: func(gc *git.Client) sshRequestFunc {
			return gc.SSHUploadPack
		},
	}
}

// NewCellsPushCommand builds a Cells SSH-over-HTTP receive-pack command.
func NewCellsPushCommand(cfg *config.Config, rw *readwriter.ReadWriter, args *commandargs.Shell, resp *accessverifier.Response) *CellsCommand {
	return &CellsCommand{
		Config:     cfg,
		ReadWriter: rw,
		Args:       args,
		Response:   resp,
		Operation:  "receive-pack",
		RequestFn: func(gc *git.Client) sshRequestFunc {
			return gc.SSHReceivePack
		},
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

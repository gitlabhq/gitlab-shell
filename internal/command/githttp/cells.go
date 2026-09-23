package githttp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path"

	"gitlab.com/gitlab-org/labkit/v2/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
)

const shellJWTHeaderName = "Gitlab-Shell-Api-Request" // #nosec G101

// signShellJWT is a variable so tests can observe when tokens are signed.
var signShellJWT = client.SignShellJWT

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

// Execute runs a Cells SSH-over-HTTP upload-pack request. Upload-pack is
// client-speaks-first, so pull can use a single request.
func (c *CellsPullCommand) Execute(ctx context.Context) error {
	log.FromContext(ctx).InfoContext(ctx, "Cells: using SSH-over-HTTP upload-pack",
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

// Execute runs a Cells SSH-over-HTTP receive-pack request. Receive-pack is
// server-speaks-first, so push must advertise before waiting for SSH input.
func (c *CellsPushCommand) Execute(ctx context.Context) error {
	log.FromContext(ctx).InfoContext(ctx, "Cells: using SSH-over-HTTP receive-pack",
		slog.String("cell_address", c.Response.CellAddress))

	gitClient, err := buildCellsGitClient(c.Config, c.Response, c.Args)
	if err != nil {
		return err
	}

	if advertisementErr := c.forwardAdvertisement(ctx, gitClient); advertisementErr != nil {
		return fmt.Errorf("cells routing: receive-pack advertisement: %w", advertisementErr)
	}

	return c.forwardPush(ctx, gitClient)
}

func (c *CellsPushCommand) forwardPush(ctx context.Context, gitClient *git.Client) error {
	clientInput, err := c.waitForPushInput(ctx)
	if err != nil {
		return err
	}
	if clientInput == nil {
		return nil
	}

	return c.forwardReceivePack(ctx, gitClient, clientInput)
}

func (c *CellsPushCommand) waitForPushInput(ctx context.Context) (*bufio.Reader, error) {
	clientInput := bufio.NewReader(c.ReadWriter.In)
	// Open the streaming request once the client begins sending. The Cell's
	// receive-pack validates command and pack framing.
	_, err := clientInput.Peek(1)
	if err == nil {
		return clientInput, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if errors.Is(err, io.EOF) {
		return nil, nil
	}

	return nil, fmt.Errorf("cells routing: waiting for receive-pack input: %w", err)
}

func (c *CellsPushCommand) forwardReceivePack(ctx context.Context, gitClient *git.Client, clientInput io.Reader) error {
	response, err := gitClient.SSHReceivePack(ctx, clientInput)
	if err != nil {
		return fmt.Errorf("cells routing: receive-pack response: %w", err)
	}
	defer response.Body.Close() //nolint:errcheck

	if err := copyAdvertisement(io.Discard, response.Body); err != nil {
		return fmt.Errorf("cells routing: receive-pack response advertisement: %w", err)
	}

	if _, err := io.Copy(c.ReadWriter.Out, response.Body); err != nil {
		return fmt.Errorf("cells routing: receive-pack response: %w", err)
	}

	return nil
}

func (c *CellsPushCommand) forwardAdvertisement(ctx context.Context, gitClient *git.Client) error {
	// A 0000 body makes command-less receive-pack exit cleanly after advertising;
	// an empty body appends fatal output and Internal Server Error instead.
	response, err := gitClient.SSHReceivePack(ctx, bytes.NewReader(pktline.PktFlush()))
	if err != nil {
		return err
	}
	defer response.Body.Close() //nolint:errcheck

	return copyAdvertisement(c.ReadWriter.Out, response.Body)
}

// copyAdvertisement leaves response bytes after the terminating flush unread.
func copyAdvertisement(w io.Writer, r io.Reader) error {
	for {
		packet, err := pktline.ReadPacket(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("advertisement ended before flush: %w", err)
			}

			return err
		}

		if _, err := w.Write(packet); err != nil {
			return err
		}

		if pktline.IsFlush(packet) {
			return nil
		}
	}
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

	headers := map[string]string{}
	if args.Env.GitProtocolVersion != "" {
		headers["Git-Protocol"] = args.Env.GitProtocolVersion
	}

	// The Shell JWT lives for one minute, but a push's second request can start
	// minutes later (e.g. after an LFS pre-push upload), so sign per request.
	shellJWTHeader := func() (map[string]string, error) {
		shellJWT, err := signShellJWT(cfg.Secret, response.UserID)
		if err != nil {
			return nil, fmt.Errorf("cells routing: generating Shell JWT: %w", err)
		}

		return map[string]string{shellJWTHeaderName: shellJWT}, nil
	}

	return &git.Client{URL: repoURL, Headers: headers, HeaderFunc: shellJWTHeader}, nil
}

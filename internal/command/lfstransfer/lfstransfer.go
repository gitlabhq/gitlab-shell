package lfstransfer

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/git-lfs-transfer/transfer"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/lfsauthenticate"
	"gitlab.com/gitlab-org/labkit/fields"
)

var (
	capabilities = []string{
		"version=1",
		"locking",
	}
)

// Command handles git-lfs-transfer operations
type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

// Execute runs the git-lfs-transfer command
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	args := c.Args.SSHArgs
	if len(args) != 3 {
		return ctx, disallowedcommand.Error
	}

	// e.g. git-lfs-transfer user/repo.git download
	repo := args[1]
	operation := args[2]

	action, err := actionFromOperation(operation)
	if err != nil {
		return ctx, err
	}

	accessResponse, err := c.verifyAccess(ctx, action, repo)
	if err != nil {
		return ctx, err
	}

	ctxWithLogData := context.WithValue(ctx, command.LogDataKey, command.NewLogData(
		accessResponse.Gitaly.Repo.GlProjectPath,
		accessResponse.Username,
		accessResponse.ProjectID,
		accessResponse.RootNamespaceID,
	))

	slog.InfoContext(ctx, "processing action", slog.Any("action", action))
	auth, err := c.authenticate(ctx, operation, repo, accessResponse.UserID)
	if err != nil {
		return ctxWithLogData, err
	}

	return c.processTransfer(ctxWithLogData, operation, action, auth)
}

func (c *Command) processTransfer(ctx context.Context, operation string, action commandargs.CommandType, auth *GitlabAuthentication) (context.Context, error) {
	backend, err := NewGitlabBackend(ctx, c.Config, c.Args, auth)
	if err != nil {
		return ctx, err
	}

	handler := transfer.NewPktline(c.ReadWriter.In, c.ReadWriter.Out)

	if err := c.sendCapabilities(ctx, handler); err != nil {
		return ctx, err
	}

	p := transfer.NewProcessor(handler, backend)
	defer slog.InfoContext(ctx, "done processing commands", slog.Any("action", action))

	switch operation {
	case transfer.DownloadOperation:
		return ctx, p.ProcessCommands(transfer.DownloadOperation)
	case transfer.UploadOperation:
		return ctx, p.ProcessCommands(transfer.UploadOperation)
	default:
		return ctx, fmt.Errorf("unknown operation %q", operation)
	}
}

func (c *Command) sendCapabilities(ctx context.Context, handler *transfer.Pktline) error {
	for _, cap := range capabilities {
		if err := handler.WritePacketText(cap); err != nil {
			slog.ErrorContext(ctx, "error sending capability", slog.String(fields.ErrorMessage, err.Error()),
				slog.String("capability", cap))
		}
	}

	if err := handler.WriteFlush(); err != nil {
		slog.ErrorContext(ctx, "error flushing capabilities", slog.String(fields.ErrorMessage, err.Error()))
		return err
	}

	return nil
}

func actionFromOperation(operation string) (commandargs.CommandType, error) {
	var action commandargs.CommandType

	switch operation {
	case transfer.DownloadOperation:
		action = commandargs.UploadPack
	case transfer.UploadOperation:
		action = commandargs.ReceivePack
	default:
		return "", disallowedcommand.Error
	}

	return action, nil
}

func (c *Command) verifyAccess(ctx context.Context, action commandargs.CommandType, repo string) (*accessverifier.Response, error) {
	cmd := accessverifier.Command{
		Config:     c.Config,
		Args:       c.Args,
		ReadWriter: c.ReadWriter,
	}

	return cmd.Verify(ctx, action, repo)
}

func (c *Command) authenticate(ctx context.Context, operation string, repo string, userID string) (*GitlabAuthentication, error) {
	client, err := lfsauthenticate.NewClient(c.Config, c.Args)
	if err != nil {
		return nil, err
	}

	response, err := client.Authenticate(ctx, operation, repo, userID)
	if err != nil {
		return nil, err
	}

	basicAuth := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", response.Username, response.LfsToken))))
	return &GitlabAuthentication{
		href: fmt.Sprintf("%s/info/lfs", response.RepoPath),
		auth: basicAuth,
	}, nil
}

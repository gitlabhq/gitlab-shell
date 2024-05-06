package lfstransfer

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/charmbracelet/git-lfs-transfer/transfer"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/lfsauthenticate"
	"gitlab.com/gitlab-org/labkit/log"
)

var (
	capabilities = []string{
		"version=1",
	}
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	args := c.Args.SshArgs
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

	ctxWithLogData := context.WithValue(ctx, "logData", command.NewLogData(
		accessResponse.Gitaly.Repo.GlProjectPath,
		accessResponse.Username,
		accessResponse.ProjectID,
		accessResponse.RootNamespaceID,
	))

	auth, err := c.authenticate(ctx, operation, repo, accessResponse.UserID)
	if err != nil {
		return ctxWithLogData, err
	}

	logger := NewWrappedLoggerForGitLFSTransfer(ctxWithLogData)

	backend, err := NewGitlabBackend(ctxWithLogData, c.Config, c.Args, auth)
	if err != nil {
		return ctxWithLogData, err
	}

	handler := transfer.NewPktline(c.ReadWriter.In, c.ReadWriter.Out, logger)

	for _, cap := range capabilities {
		if err := handler.WritePacketText(cap); err != nil {
			log.WithContextFields(ctxWithLogData, log.Fields{"capability": cap}).WithError(err).Error("error sending capability")
		}
	}

	if err := handler.WriteFlush(); err != nil {
		log.WithContextFields(ctxWithLogData, log.Fields{}).WithError(err).Error("error flushing capabilities")
	}

	p := transfer.NewProcessor(handler, backend, logger)
	defer log.WithContextFields(ctxWithLogData, log.Fields{}).Info("done processing commands")
	switch operation {
	case transfer.DownloadOperation:
		return ctxWithLogData, p.ProcessCommands(transfer.DownloadOperation)
	case transfer.UploadOperation:
		return ctxWithLogData, p.ProcessCommands(transfer.UploadOperation)
	default:
		return ctxWithLogData, fmt.Errorf("unknown operation %q", operation)
	}
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
	cmd := accessverifier.Command{c.Config, c.Args, c.ReadWriter}

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

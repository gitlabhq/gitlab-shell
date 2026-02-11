// Package lfsauthenticate provides functionality for authenticating Git LFS requests
package lfsauthenticate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/lfsauthenticate"
	"gitlab.com/gitlab-org/labkit/fields"
)

const (
	downloadOperation = "download"
	uploadOperation   = "upload"
)

// Command represents the LFS authentication command
type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

// PayloadHeader represents the header of the LFS payload
type PayloadHeader struct {
	Auth string `json:"Authorization"`
}

// Payload represents the LFS payload
type Payload struct {
	Header    PayloadHeader `json:"header"`
	Href      string        `json:"href"`
	ExpiresIn int           `json:"expires_in,omitempty"`
}

type logInfo struct{}

// Execute executes the LFS authentication command
func (c *Command) Execute(ctx context.Context) (context.Context, error) {
	args := c.Args.SSHArgs
	if len(args) < 3 {
		return ctx, disallowedcommand.Error
	}

	// e.g. git-lfs-authenticate user/repo.git download
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

	logData := command.NewLogData(
		accessResponse.Gitaly.Repo.GlProjectPath,
		accessResponse.Username,
		accessResponse.ProjectID,
		accessResponse.RootNamespaceID,
	)
	ctxWithLogData := context.WithValue(ctx, logInfo{}, logData)

	payload, err := c.authenticate(ctx, operation, repo, accessResponse.UserID)
	if err != nil {
		// return nothing just like Ruby's GitlabShell#lfs_authenticate does
		slog.DebugContext(ctx, "lfsauthenticate: execute: LFS authentication failed",
			slog.String("operation", operation),
			slog.String("gl_repository", repo),
			slog.String("user_id", accessResponse.UserID),
			slog.String(fields.ErrorMessage, err.Error()),
		)

		return ctxWithLogData, nil
	}

	if _, err := fmt.Fprintf(c.ReadWriter.Out, "%s\n", payload); err != nil {
		return ctxWithLogData, err
	}

	return ctxWithLogData, nil
}

func actionFromOperation(operation string) (commandargs.CommandType, error) {
	var action commandargs.CommandType

	switch operation {
	case downloadOperation:
		action = commandargs.UploadPack
	case uploadOperation:
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

func (c *Command) authenticate(ctx context.Context, operation string, repo, userID string) ([]byte, error) {
	client, err := lfsauthenticate.NewClient(c.Config, c.Args)
	if err != nil {
		return nil, err
	}

	response, err := client.Authenticate(ctx, operation, repo, userID)
	if err != nil {
		return nil, err
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(response.Username + ":" + response.LfsToken))
	payload := &Payload{
		Header:    PayloadHeader{Auth: "Basic " + basicAuth},
		Href:      response.RepoPath + "/info/lfs",
		ExpiresIn: response.ExpiresIn,
	}

	return json.Marshal(payload)
}

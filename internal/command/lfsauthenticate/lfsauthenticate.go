package lfsauthenticate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/lfsauthenticate"
)

const (
	downloadOperation = "download"
	uploadOperation   = "upload"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.Shell
	ReadWriter *readwriter.ReadWriter
}

type PayloadHeader struct {
	Auth string `json:"Authorization"`
}

type Payload struct {
	Header    PayloadHeader `json:"header"`
	Href      string        `json:"href"`
	ExpiresIn int           `json:"expires_in,omitempty"`
}

func (c *Command) Execute(ctx context.Context) error {
	args := c.Args.SshArgs
	if len(args) < 3 {
		return disallowedcommand.Error
	}

	// e.g. git-lfs-authenticate user/repo.git download
	repo := args[1]
	operation := args[2]

	action, err := actionFromOperation(operation)
	if err != nil {
		return err
	}

	accessResponse, err := c.verifyAccess(ctx, action, repo)
	if err != nil {
		return err
	}

	payload, err := c.authenticate(ctx, operation, repo, accessResponse.UserId)
	if err != nil {
		// return nothing just like Ruby's GitlabShell#lfs_authenticate does
		log.WithContextFields(
			ctx,
			log.Fields{"operation": operation, "repo": repo, "user_id": accessResponse.UserId},
		).WithError(err).Debug("lfsauthenticate: execute: LFS authentication failed")

		return nil
	}

	fmt.Fprintf(c.ReadWriter.Out, "%s\n", payload)

	return nil
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
	cmd := accessverifier.Command{c.Config, c.Args, c.ReadWriter}

	return cmd.Verify(ctx, action, repo)
}

func (c *Command) authenticate(ctx context.Context, operation string, repo, userId string) ([]byte, error) {
	client, err := lfsauthenticate.NewClient(c.Config, c.Args)
	if err != nil {
		return nil, err
	}

	response, err := client.Authenticate(ctx, operation, repo, userId)
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

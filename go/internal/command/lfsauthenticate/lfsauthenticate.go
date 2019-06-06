package lfsauthenticate

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/gitlabnet/lfsauthenticate"
)

const (
	downloadAction = "download"
	uploadAction   = "upload"
)

type Command struct {
	Config     *config.Config
	Args       *commandargs.CommandArgs
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

func (c *Command) Execute() error {
	args := c.Args.SshArgs
	if len(args) < 3 {
		return disallowedcommand.Error
	}

	repo := args[1]
	action, err := actionToCommandType(args[2])
	if err != nil {
		return err
	}

	accessResponse, err := c.verifyAccess(action, repo)
	if err != nil {
		return err
	}

	payload, err := c.authenticate(action, repo, accessResponse.UserId)
	if err != nil {
		// return nothing just like Ruby's GitlabShell#lfs_authenticate does
		return nil
	}

	fmt.Fprintf(c.ReadWriter.Out, "%s\n", payload)

	return nil
}

func actionToCommandType(action string) (commandargs.CommandType, error) {
	var accessAction commandargs.CommandType
	switch action {
	case downloadAction:
		accessAction = commandargs.UploadPack
	case uploadAction:
		accessAction = commandargs.ReceivePack
	default:
		return "", disallowedcommand.Error
	}

	return accessAction, nil
}

func (c *Command) verifyAccess(action commandargs.CommandType, repo string) (*accessverifier.Response, error) {
	cmd := accessverifier.Command{c.Config, c.Args, c.ReadWriter}

	return cmd.Verify(action, repo)
}

func (c *Command) authenticate(action commandargs.CommandType, repo, userId string) ([]byte, error) {
	client, err := lfsauthenticate.NewClient(c.Config, c.Args)
	if err != nil {
		return nil, err
	}

	response, err := client.Authenticate(action, repo, userId)
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

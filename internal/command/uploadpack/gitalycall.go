package uploadpack

import (
	"context"
	"os"

	"google.golang.org/grpc"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/client"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/handler"
)

func (c *Command) performGitalyCall(response *accessverifier.Response) error {
	gc := &handler.GitalyCommand{
		Config:      c.Config,
		ServiceName: string(commandargs.UploadPack),
		Address:     response.Gitaly.Address,
		Token:       response.Gitaly.Token,
		Features:    response.Gitaly.Features,
	}

	request := &pb.SSHUploadPackRequest{
		Repository:       &response.Gitaly.Repo,
		GitProtocol:      os.Getenv(commandargs.GitProtocolEnv),
		GitConfigOptions: response.GitConfigOptions,
	}

	fields := log.Fields{
		"command":       "git-upload-pack",
		"glProjectPath": request.Repository.GlProjectPath,
		"glRepository":  request.Repository.GlRepository,
		"userId":        response.UserId,
		"userName":      response.Username,
		"gitProtocol":   request.GitProtocol,
	}

	log.WithFields(fields).Info("executing git command")

	return gc.RunGitalyCommand(func(ctx context.Context, conn *grpc.ClientConn) (int32, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		rw := c.ReadWriter
		return client.UploadPack(ctx, conn, rw.In, rw.Out, rw.ErrOut, request)
	})
}

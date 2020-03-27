package receivepack

import (
	"context"
	"os"

	"google.golang.org/grpc"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/client"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/handler"
)

func (c *Command) performGitalyCall(response *accessverifier.Response) error {
	gc := &handler.GitalyCommand{
		Config:      c.Config,
		ServiceName: string(commandargs.ReceivePack),
		Address:     response.Gitaly.Address,
		Token:       response.Gitaly.Token,
		Features:    response.Gitaly.Features,
	}

	request := &pb.SSHReceivePackRequest{
		Repository:       &response.Gitaly.Repo,
		GlId:             response.UserId,
		GlRepository:     response.Repo,
		GlUsername:       response.Username,
		GitProtocol:      os.Getenv(commandargs.GitProtocolEnv),
		GitConfigOptions: response.GitConfigOptions,
	}

	fields := log.Fields{
		"command":         "git-receive-pack",
		"gl_project_path": request.Repository.GlProjectPath,
		"gl_repository":   request.Repository.GlRepository,
		"user_id":         response.UserId,
		"username":        response.Username,
		"git_protocol":    request.GitProtocol,
	}

	log.WithFields(fields).Info("executing git command")

	return gc.RunGitalyCommand(func(ctx context.Context, conn *grpc.ClientConn) (int32, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		rw := c.ReadWriter
		return client.ReceivePack(ctx, conn, rw.In, rw.Out, rw.ErrOut, request)
	})
}

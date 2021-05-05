package receivepack

import (
	"context"

	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitaly/client"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/handler"
)

func (c *Command) performGitalyCall(ctx context.Context, response *accessverifier.Response) error {
	gc := &handler.GitalyCommand{
		Config:      c.Config,
		ServiceName: string(commandargs.ReceivePack),
		Address:     response.Gitaly.Address,
		Token:       response.Gitaly.Token,
		Features:    response.Gitaly.Features,
	}

	request := &pb.SSHReceivePackRequest{
		Repository:       &response.Gitaly.Repo,
		GlId:             response.Who,
		GlRepository:     response.Repo,
		GlUsername:       response.Username,
		GitProtocol:      c.Args.Env.GitProtocolVersion,
		GitConfigOptions: response.GitConfigOptions,
	}

	return gc.RunGitalyCommand(ctx, func(ctx context.Context, conn *grpc.ClientConn) (int32, error) {
		ctx, cancel := gc.PrepareContext(ctx, request.Repository, response, c.Args.Env)
		defer cancel()

		rw := c.ReadWriter
		return client.ReceivePack(ctx, conn, rw.In, rw.Out, rw.ErrOut, request)
	})
}

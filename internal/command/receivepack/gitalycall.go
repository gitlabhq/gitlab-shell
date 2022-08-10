package receivepack

import (
	"context"

	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitaly/v15/client"
	pb "gitlab.com/gitlab-org/gitaly/v15/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/handler"
)

func (c *Command) performGitalyCall(ctx context.Context, response *accessverifier.Response) error {
	gc := handler.NewGitalyCommand(c.Config, string(commandargs.ReceivePack), response)

	request := &pb.SSHReceivePackRequest{
		Repository:       &response.Gitaly.Repo,
		GlId:             response.Who,
		GlRepository:     response.Repo,
		GlUsername:       response.Username,
		GitProtocol:      c.Args.Env.GitProtocolVersion,
		GitConfigOptions: response.GitConfigOptions,
	}

	return gc.RunGitalyCommand(ctx, func(ctx context.Context, conn *grpc.ClientConn) (int32, error) {
		ctx, cancel := gc.PrepareContext(ctx, request.Repository, c.Args.Env)
		defer cancel()

		rw := c.ReadWriter
		return client.ReceivePack(ctx, conn, rw.In, rw.Out, rw.ErrOut, request)
	})
}

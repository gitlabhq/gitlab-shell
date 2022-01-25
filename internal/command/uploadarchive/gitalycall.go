package uploadarchive

import (
	"context"

	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitaly/v14/client"
	pb "gitlab.com/gitlab-org/gitaly/v14/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/handler"
)

func (c *Command) performGitalyCall(ctx context.Context, response *accessverifier.Response) error {
	gc := &handler.GitalyCommand{
		Config:      c.Config,
		ServiceName: string(commandargs.UploadArchive),
		Address:     response.Gitaly.Address,
		Token:       response.Gitaly.Token,
		Features:    response.Gitaly.Features,
	}

	request := &pb.SSHUploadArchiveRequest{Repository: &response.Gitaly.Repo}

	return gc.RunGitalyCommand(ctx, func(ctx context.Context, conn *grpc.ClientConn, registry *client.SidechannelRegistry) (int32, error) {
		ctx, cancel := gc.PrepareContext(ctx, request.Repository, response, c.Args.Env)
		defer cancel()

		rw := c.ReadWriter
		return client.UploadArchive(ctx, conn, rw.In, rw.Out, rw.ErrOut, request)
	})
}

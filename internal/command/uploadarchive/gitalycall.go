package uploadarchive

import (
	"context"

	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitaly/v15/client"
	pb "gitlab.com/gitlab-org/gitaly/v15/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/handler"
)

func (c *Command) performGitalyCall(ctx context.Context, response *accessverifier.Response) error {
	gc := handler.NewGitalyCommand(c.Config, string(commandargs.UploadArchive), response)

	request := &pb.SSHUploadArchiveRequest{Repository: &response.Gitaly.Repo}

	return gc.RunGitalyCommand(ctx, func(ctx context.Context, conn *grpc.ClientConn) (int32, error) {
		ctx, cancel := gc.PrepareContext(ctx, request.Repository, c.Args.Env)
		defer cancel()

		rw := c.ReadWriter
		return client.UploadArchive(ctx, conn, rw.In, rw.Out, rw.ErrOut, request)
	})
}

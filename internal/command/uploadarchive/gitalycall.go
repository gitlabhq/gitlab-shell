package uploadarchive

import (
	"context"

	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitaly/client"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/handler"
)

func (c *Command) performGitalyCall(response *accessverifier.Response) error {
	gc := &handler.GitalyCommand{
		Config:      c.Config,
		ServiceName: string(commandargs.UploadArchive),
		Address:     response.Gitaly.Address,
		Token:       response.Gitaly.Token,
		Features:    response.Gitaly.Features,
	}

	request := &pb.SSHUploadArchiveRequest{Repository: &response.Gitaly.Repo}

	return gc.RunGitalyCommand(func(ctx context.Context, conn *grpc.ClientConn) (int32, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		rw := c.ReadWriter
		return client.UploadArchive(ctx, conn, rw.In, rw.Out, rw.ErrOut, request)
	})
}

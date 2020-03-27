package uploadarchive

import (
	"context"

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
		ServiceName: string(commandargs.UploadArchive),
		Address:     response.Gitaly.Address,
		Token:       response.Gitaly.Token,
		Features:    response.Gitaly.Features,
	}

	request := &pb.SSHUploadArchiveRequest{Repository: &response.Gitaly.Repo}

	fields := log.Fields{
		"command":       "git-upload-archive",
		"glProjectPath": request.Repository.GlProjectPath,
		"glRepository":  request.Repository.GlRepository,
		"userId":        response.UserId,
		"userName":      response.Username,
	}

	log.WithFields(fields).Info("executing git command")

	return gc.RunGitalyCommand(func(ctx context.Context, conn *grpc.ClientConn) (int32, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		rw := c.ReadWriter
		return client.UploadArchive(ctx, conn, rw.In, rw.Out, rw.ErrOut, request)
	})
}

// Package uploadpack provides functionality for handling upload-pack command
package uploadpack

import (
	"context"

	"google.golang.org/grpc"

	"gitlab.com/gitlab-org/gitaly/v16/client"
	pb "gitlab.com/gitlab-org/gitaly/v16/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/handler"
)

func (c *Command) performGitalyCall(ctx context.Context, response *accessverifier.Response) (*pb.PackfileNegotiationStatistics, error) {
	gc := handler.NewGitalyCommand(c.Config, string(commandargs.UploadPack), response)

	request := &pb.SSHUploadPackWithSidechannelRequest{
		Repository:       &response.Gitaly.Repo,
		GitProtocol:      c.Args.Env.GitProtocolVersion,
		GitConfigOptions: response.GitConfigOptions,
	}

	var stats *pb.PackfileNegotiationStatistics
	err := gc.RunGitalyCommand(ctx, func(ctx context.Context, conn *grpc.ClientConn) (int32, error) {
		ctx, cancel := gc.PrepareContext(ctx, request.Repository, c.Args.Env)
		defer cancel()

		registry := c.Config.GitalyClient.SidechannelRegistry
		rw := c.ReadWriter

		var (
			result client.UploadPackResult
			err    error
		)
		result, err = client.UploadPackWithSidechannelWithResult(ctx, conn, registry, rw.In, rw.Out, rw.ErrOut, request)
		if err == nil {
			stats = result.PackfileNegotiationStatistics
		}
		return result.ExitCode, err
	})

	return stats, err
}

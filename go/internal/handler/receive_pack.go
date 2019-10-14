package handler

import (
	"context"
	"os"

	"gitlab.com/gitlab-org/gitaly/client"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"google.golang.org/grpc"
)

// ReceivePack issues a Gitaly receive-pack rpc to the provided address
func ReceivePack(ctx context.Context, conn *grpc.ClientConn, request *pb.SSHReceivePackRequest) (int32, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return client.ReceivePack(ctx, conn, os.Stdin, os.Stdout, os.Stderr, request)
}

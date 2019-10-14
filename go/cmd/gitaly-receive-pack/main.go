package main

import (
	"context"
	"encoding/json"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/handler"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/logger"
	"google.golang.org/grpc"

	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

func init() {
	logger.ProgName = "gitaly-receive-pack"
}

func main() {
	handler.RunGitalyCommand(func(ctx context.Context, conn *grpc.ClientConn, requestJSON string) (int32, error) {
		request, err := deserialize(requestJSON)
		if err != nil {
			return 1, err
		}

		return handler.ReceivePack(ctx, conn, request)
	})
}

func deserialize(requestJSON string) (*pb.SSHReceivePackRequest, error) {
	var request pb.SSHReceivePackRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		return nil, err
	}
	return &request, nil
}

package handler

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/grpc"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
	"gitlab.com/gitlab-org/gitaly/auth"
	"gitlab.com/gitlab-org/gitaly/client"
)

func ReceivePack(gitalyAddress string, request *pb.SSHReceivePackRequest) (int32, error) {
	if gitalyAddress == "" {
		return 0, fmt.Errorf("no gitaly_address given")
	}

	connOpts := client.DefaultDialOpts
	if token := os.Getenv("GITALY_TOKEN"); token != "" {
		connOpts = append(client.DefaultDialOpts, grpc.WithPerRPCCredentials(gitalyauth.RPCCredentials(token)))
	}

	conn, err := client.Dial(gitalyAddress, connOpts)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return client.ReceivePack(ctx, conn, os.Stdin, os.Stdout, os.Stderr, request)
}

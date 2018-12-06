package handler

import (
	"context"
	"fmt"
	"os"

	"gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/client"
)

func ReceivePack(gitalyAddress string, request *gitalypb.SSHReceivePackRequest) (int32, error) {
	if gitalyAddress == "" {
		return 0, fmt.Errorf("no gitaly_address given")
	}

	conn, err := client.Dial(gitalyAddress, dialOpts())
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return client.ReceivePack(ctx, conn, os.Stdin, os.Stdout, os.Stderr, request)
}

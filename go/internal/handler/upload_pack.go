package handler

import (
	"context"
	"fmt"
	"os"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
	"gitlab.com/gitlab-org/gitaly/client"
)

func UploadPack(gitalyAddress string, request *pb.SSHUploadPackRequest) (int32, error) {
	if gitalyAddress == "" {
		return -1, fmt.Errorf("no gitaly_address given")
	}

	conn, err := client.Dial(gitalyAddress, client.DefaultDialOpts)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	return client.UploadPack(ctx, conn, os.Stdin, os.Stdout, os.Stderr, request)
}

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/handler"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/logger"

	pb "gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
)

func init() {
	logger.ProgName = "gitaly-upload-archive"
}

type uploadArchiveHandler func(gitalyAddress string, request *pb.SSHUploadArchiveRequest) (int32, error)

func main() {
	if err := handler.Prepare(); err != nil {
		logger.Fatal("preparation failed", err)
	}

	code, err := uploadArchive(handler.UploadArchive, os.Args)

	if err != nil {
		logger.Fatal("upload-archive failed", err)
	}

	os.Exit(int(code))
}

func uploadArchive(handler uploadArchiveHandler, args []string) (int32, error) {
	if n := len(args); n != 3 {
		return 0, fmt.Errorf("wrong number of arguments: expected 2 arguments, got %v", args)
	}

	var request pb.SSHUploadArchiveRequest
	if err := json.Unmarshal([]byte(args[2]), &request); err != nil {
		return 0, fmt.Errorf("unmarshaling request json failed: %v", err)
	}

	return handler(args[1], &request)
}

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
	logger.ProgName = "gitaly-upload-pack"
}

func main() {
	if err := handler.Prepare(); err != nil {
		logger.Fatal("preparation failed", err)
	}

	if n := len(os.Args); n != 3 {
		logger.Fatal("wrong number of arguments", fmt.Errorf("expected 2 arguments, got %v", os.Args))
	}

	var request pb.SSHUploadPackRequest
	if err := json.Unmarshal([]byte(os.Args[2]), &request); err != nil {
		logger.Fatal("unmarshaling request json failed", err)
	}

	code, err := handler.UploadPack(os.Args[1], &request)
	if err != nil {
		logger.Fatal("upload-pack failed", err)
	}
	os.Exit(int(code))
}

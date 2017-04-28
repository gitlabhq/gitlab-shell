package main

import (
	"encoding/json"
	"os"

	"gitlab.com/gitlab-org/gitlab-shell/go/internal/handler"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/logger"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
)

func init() {
	logger.ProgName = "gitaly-upload-pack"
}

func main() {
	if err := handler.Prepare(); err != nil {
		logger.Fatal(err)
	}

	var request pb.SSHUploadPackRequest
	if err := json.Unmarshal([]byte(os.Args[2]), &request); err != nil {
		logger.Fatal(err)
	}

	if err := handler.UploadPack(os.Args[1], &request); err != nil {
		logger.Fatal(err)
	}
}

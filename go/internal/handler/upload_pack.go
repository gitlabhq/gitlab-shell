package handler

import (
	"fmt"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
)

func UploadPack(gitalyAddress string, request *pb.SSHUploadPackRequest) error {
	repoPath := request.Repository.Path
	if repoPath == "" {
		return fmt.Errorf("empty path in repository message")
	}

	return execCommand("git-upload-pack", repoPath)
}

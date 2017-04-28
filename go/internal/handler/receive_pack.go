package handler

import (
	"fmt"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
)

func ReceivePack(gitalyAddress string, request *pb.SSHReceivePackRequest) error {
	repoPath := request.Repository.Path
	if repoPath == "" {
		return fmt.Errorf("empty path in repository message")
	}

	return execCommand("git-receive-pack", repoPath)
}

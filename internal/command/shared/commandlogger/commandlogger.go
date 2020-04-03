package commandlogger

import (
	log "github.com/sirupsen/logrus"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
)

func Log(command string, repository *pb.Repository, response *accessverifier.Response, protocol string) {
	fields := log.Fields{
		"command":       command,
		"glProjectPath": repository.GlProjectPath,
		"glRepository":  repository.GlRepository,
		"userId":        response.UserId,
		"userName":      response.Username,
		"gitProtocol":   protocol,
	}

	log.WithFields(fields).Info("executing git command")
}

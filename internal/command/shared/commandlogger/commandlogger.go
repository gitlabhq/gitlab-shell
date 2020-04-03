package commandlogger

import (
	log "github.com/sirupsen/logrus"
	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
)

func Log(command string, repository *pb.Repository, response *accessverifier.Response, protocol string) {
	fields := log.Fields{
		"command":       command,
		"gl_project_path": repository.GlProjectPath,
		"gl_repository":  repository.GlRepository,
		"user_id":        response.UserId,
		"username":      response.Username,
		"git_protocol":   protocol,
	}

	log.WithFields(fields).Info("executing git command")
}

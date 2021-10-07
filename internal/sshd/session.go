package sshd

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"gitlab.com/gitlab-org/labkit/log"
	"golang.org/x/crypto/ssh"

	shellCmd "gitlab.com/gitlab-org/gitlab-shell/cmd/gitlab-shell/command"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
)

type session struct {
	// State set up by the connection
	cfg         *config.Config
	channel     ssh.Channel
	gitlabKeyId string
	remoteAddr  string

	// State managed by the session
	execCmd            string
	gitProtocolVersion string
}

type execRequest struct {
	Command string
}

type envRequest struct {
	Name  string
	Value string
}

type exitStatusReq struct {
	ExitStatus uint32
}

func (s *session) handle(ctx context.Context, requests <-chan *ssh.Request) {
	ctxlog := log.ContextLogger(ctx)

	ctxlog.Debug("session: handle: entering request loop")

	for req := range requests {
		sessionLog := ctxlog.WithFields(log.Fields{
			"bytesize":   len(req.Payload),
			"type":       req.Type,
			"want_reply": req.WantReply,
		})
		sessionLog.Debug("session: handle: request received")

		var shouldContinue bool
		switch req.Type {
		case "env":
			shouldContinue = s.handleEnv(ctx, req)
		case "exec":
			shouldContinue = s.handleExec(ctx, req)
		case "shell":
			shouldContinue = false
			s.exit(ctx, s.handleShell(ctx, req))
		default:
			// Ignore unknown requests but don't terminate the session
			shouldContinue = true
			if req.WantReply {
				req.Reply(false, []byte{})
			}
		}

		sessionLog.WithField("should_continue", shouldContinue).Debug("session: handle: request processed")

		if !shouldContinue {
			s.channel.Close()
			break
		}
	}

	ctxlog.Debug("session: handle: exiting request loop")
}

func (s *session) handleEnv(ctx context.Context, req *ssh.Request) bool {
	var accepted bool
	var envRequest envRequest

	if err := ssh.Unmarshal(req.Payload, &envRequest); err != nil {
		log.ContextLogger(ctx).WithError(err).Error("session: handleEnv: failed to unmarshal request")
		return false
	}

	switch envRequest.Name {
	case sshenv.GitProtocolEnv:
		s.gitProtocolVersion = envRequest.Value
		accepted = true
	default:
		// Client requested a forbidden envvar, nothing to do
	}

	if req.WantReply {
		req.Reply(accepted, []byte{})
	}

	log.WithContextFields(
		ctx, log.Fields{"accepted": accepted, "env_request": envRequest},
	).Debug("session: handleEnv: processed")

	return true
}

func (s *session) handleExec(ctx context.Context, req *ssh.Request) bool {
	var execRequest execRequest
	if err := ssh.Unmarshal(req.Payload, &execRequest); err != nil {
		return false
	}

	s.execCmd = execRequest.Command

	s.exit(ctx, s.handleShell(ctx, req))

	return false
}

func (s *session) handleShell(ctx context.Context, req *ssh.Request) uint32 {
	if req.WantReply {
		req.Reply(true, []byte{})
	}

	env := sshenv.Env{
		IsSSHConnection:    true,
		OriginalCommand:    s.execCmd,
		GitProtocolVersion: s.gitProtocolVersion,
		RemoteAddr:         s.remoteAddr,
	}

	rw := &readwriter.ReadWriter{
		Out:    s.channel,
		In:     s.channel,
		ErrOut: s.channel.Stderr(),
	}

	cmd, err := shellCmd.NewWithKey(s.gitlabKeyId, env, s.cfg, rw)
	if err != nil {
		if !errors.Is(err, disallowedcommand.Error) {
			s.toStderr(ctx, "Failed to parse command: %v\n", err.Error())
		}
		s.toStderr(ctx, "Unknown command: %v\n", s.execCmd)
		return 128
	}

	cmdName := reflect.TypeOf(cmd).String()
	ctxlog := log.ContextLogger(ctx)
	ctxlog.WithFields(log.Fields{"env": env, "command": cmdName}).Info("session: handleShell: executing command")

	if err := cmd.Execute(ctx); err != nil {
		s.toStderr(ctx, "remote: ERROR: %v\n", err.Error())
		return 1
	}

	ctxlog.Info("session: handleShell: command executed successfully")

	return 0
}

func (s *session) toStderr(ctx context.Context, format string, args ...interface{}) {
	out := fmt.Sprintf(format, args...)
	log.WithContextFields(ctx, log.Fields{"stderr": out}).Debug("session: toStderr: output")
	fmt.Fprint(s.channel.Stderr(), out)
}

func (s *session) exit(ctx context.Context, status uint32) {
	log.WithContextFields(ctx, log.Fields{"exit_status": status}).Info("session: exit: exiting")
	req := exitStatusReq{ExitStatus: status}

	s.channel.CloseWrite()
	s.channel.SendRequest("exit-status", false, ssh.Marshal(req))
}

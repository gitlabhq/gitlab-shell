package sshd

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"gitlab.com/gitlab-org/labkit/log"
	"golang.org/x/crypto/ssh"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	shellCmd "gitlab.com/gitlab-org/gitlab-shell/v14/cmd/gitlab-shell/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/shared/disallowedcommand"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/console"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

type session struct {
	// State set up by the connection
	cfg                 *config.Config
	channel             ssh.Channel
	gitlabKeyId         string
	gitlabKrb5Principal string
	remoteAddr          string

	// State managed by the session
	execCmd            string
	gitProtocolVersion string
	started            time.Time
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

func (s *session) handle(ctx context.Context, requests <-chan *ssh.Request) error {
	ctxlog := log.ContextLogger(ctx)

	ctxlog.Debug("session: handle: entering request loop")

	var err error
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
			shouldContinue, err = s.handleEnv(ctx, req)
		case "exec":
			// The command has been executed as `ssh user@host command` or `exec` channel has been used
			// in the app implementation
			shouldContinue, err = s.handleExec(ctx, req)
		case "shell":
			// The command has been entered into the shell or `shell` channel has been used
			// in the app implementation
			shouldContinue = false
			var status uint32
			status, err = s.handleShell(ctx, req)
			s.exit(ctx, status)
		default:
			// Ignore unknown requests but don't terminate the session
			shouldContinue = true

			if req.WantReply {
				if err := req.Reply(false, []byte{}); err != nil {
					sessionLog.WithError(err).Debug("session: handle: Failed to reply")
				}
			}
		}

		sessionLog.WithField("should_continue", shouldContinue).Debug("session: handle: request processed")

		if !shouldContinue {
			s.channel.Close()
			break
		}
	}

	ctxlog.Debug("session: handle: exiting request loop")

	return err
}

func (s *session) handleEnv(ctx context.Context, req *ssh.Request) (bool, error) {
	var accepted bool
	var envRequest envRequest

	if err := ssh.Unmarshal(req.Payload, &envRequest); err != nil {
		log.ContextLogger(ctx).WithError(err).Error("session: handleEnv: failed to unmarshal request")
		return false, err
	}

	switch envRequest.Name {
	case sshenv.GitProtocolEnv:
		s.gitProtocolVersion = envRequest.Value
		accepted = true
	default:
		// Client requested a forbidden envvar, nothing to do
	}

	if req.WantReply {
		if err := req.Reply(accepted, []byte{}); err != nil {
			log.ContextLogger(ctx).WithError(err).Debug("session: handleEnv: Failed to reply")
		}
	}

	log.WithContextFields(
		ctx, log.Fields{"accepted": accepted, "env_request": envRequest},
	).Debug("session: handleEnv: processed")

	return true, nil
}

func (s *session) handleExec(ctx context.Context, req *ssh.Request) (bool, error) {
	var execRequest execRequest
	if err := ssh.Unmarshal(req.Payload, &execRequest); err != nil {
		return false, err
	}

	s.execCmd = execRequest.Command

	status, err := s.handleShell(ctx, req)
	s.exit(ctx, status)

	return false, err
}

func (s *session) handleShell(ctx context.Context, req *ssh.Request) (uint32, error) {
	ctxlog := log.ContextLogger(ctx)

	if req.WantReply {
		if err := req.Reply(true, []byte{}); err != nil {
			ctxlog.WithError(err).Debug("session: handleShell: Failed to reply")
		}
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

	var cmd command.Command
	var err error

	if s.gitlabKrb5Principal != "" {
		cmd, err = shellCmd.NewWithKrb5Principal(s.gitlabKrb5Principal, env, s.cfg, rw)
	} else {
		cmd, err = shellCmd.NewWithKey(s.gitlabKeyId, env, s.cfg, rw)
	}
	if err != nil {
		if errors.Is(err, disallowedcommand.Error) {
			s.toStderr(ctx, "ERROR: Unknown command: %v\n", s.execCmd)
		} else {
			s.toStderr(ctx, "ERROR: Failed to parse command: %v\n", err.Error())
		}

		return 128, err
	}

	cmdName := reflect.TypeOf(cmd).String()

	establishSessionDuration := time.Since(s.started).Seconds()
	ctxlog.WithFields(log.Fields{
		"env": env, "command": cmdName, "established_session_duration_s": establishSessionDuration,
	}).Info("session: handleShell: executing command")
	metrics.SshdSessionEstablishedDuration.Observe(establishSessionDuration)

	if err := cmd.Execute(ctx); err != nil {
		grpcStatus := grpcstatus.Convert(err)
		if grpcStatus.Code() != grpccodes.Internal {
			s.toStderr(ctx, "ERROR: %v\n", grpcStatus.Message())
		}

		return 1, err
	}

	ctxlog.Info("session: handleShell: command executed successfully")

	return 0, nil
}

func (s *session) toStderr(ctx context.Context, format string, args ...interface{}) {
	out := fmt.Sprintf(format, args...)
	log.WithContextFields(ctx, log.Fields{"stderr": out}).Debug("session: toStderr: output")
	console.DisplayWarningMessage(out, s.channel.Stderr())
}

func (s *session) exit(ctx context.Context, status uint32) {
	log.WithContextFields(ctx, log.Fields{"exit_status": status}).Info("session: exit: exiting")
	req := exitStatusReq{ExitStatus: status}

	s.channel.CloseWrite()
	s.channel.SendRequest("exit-status", false, ssh.Marshal(req))
}

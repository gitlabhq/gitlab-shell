package sshd

import (
	"context"
	"fmt"

	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
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
	for req := range requests {
		var shouldContinue bool
		switch req.Type {
		case "env":
			shouldContinue = s.handleEnv(req)
		case "exec":
			shouldContinue = s.handleExec(ctx, req)
		case "shell":
			shouldContinue = false
			s.exit(s.handleShell(ctx, req))
		default:
			// Ignore unknown requests but don't terminate the session
			shouldContinue = true
			if req.WantReply {
				req.Reply(false, []byte{})
			}
		}

		if !shouldContinue {
			s.channel.Close()
			break
		}
	}
}

func (s *session) handleEnv(req *ssh.Request) bool {
	var accepted bool
	var envRequest envRequest

	if err := ssh.Unmarshal(req.Payload, &envRequest); err != nil {
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

	return true
}

func (s *session) handleExec(ctx context.Context, req *ssh.Request) bool {
	var execRequest execRequest
	if err := ssh.Unmarshal(req.Payload, &execRequest); err != nil {
		return false
	}

	s.execCmd = execRequest.Command

	s.exit(s.handleShell(ctx, req))
	return false
}

func (s *session) handleShell(ctx context.Context, req *ssh.Request) uint32 {
	if req.WantReply {
		req.Reply(true, []byte{})
	}

	args := &commandargs.Shell{
		GitlabKeyId: s.gitlabKeyId,
		Env: sshenv.Env{
			IsSSHConnection:    true,
			OriginalCommand:    s.execCmd,
			GitProtocolVersion: s.gitProtocolVersion,
			RemoteAddr:         s.remoteAddr,
		},
	}

	if err := args.ParseCommand(s.execCmd); err != nil {
		s.toStderr("Failed to parse command: %v\n", err.Error())
		return 128
	}

	rw := &readwriter.ReadWriter{
		Out:    s.channel,
		In:     s.channel,
		ErrOut: s.channel.Stderr(),
	}

	cmd := command.BuildShellCommand(args, s.cfg, rw)
	if cmd == nil {
		s.toStderr("Unknown command: %v\n", args.CommandType)
		return 128
	}

	if err := cmd.Execute(ctx); err != nil {
		s.toStderr("remote: ERROR: %v\n", err.Error())
		return 1
	}

	return 0
}

func (s *session) toStderr(format string, args ...interface{}) {
	fmt.Fprintf(s.channel.Stderr(), format, args...)
}

func (s *session) exit(status uint32) {
	req := exitStatusReq{ExitStatus: status}

	s.channel.CloseWrite()
	s.channel.SendRequest("exit-status", false, ssh.Marshal(req))
}

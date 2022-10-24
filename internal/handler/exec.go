package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitaly"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"

	pb "gitlab.com/gitlab-org/gitaly/v15/proto/go/gitalypb"
	"gitlab.com/gitlab-org/labkit/log"
)

// GitalyHandlerFunc implementations are responsible for making
// an appropriate Gitaly call using the provided client and context
// and returning an error from the Gitaly call.
type GitalyHandlerFunc func(ctx context.Context, client *grpc.ClientConn) (int32, error)

type GitalyCommand struct {
	Config   *config.Config
	Response *accessverifier.Response
	Command  gitaly.Command
}

func NewGitalyCommand(cfg *config.Config, serviceName string, response *accessverifier.Response) *GitalyCommand {
	gc := gitaly.Command{
		ServiceName: serviceName,
		Address:     response.Gitaly.Address,
		Token:       response.Gitaly.Token,
	}

	return &GitalyCommand{Config: cfg, Response: response, Command: gc}
}

// processGitalyError handles errors that come back from Gitaly that may be a
// LimitError. A LimitError is returned by Gitaly when it is at its limit in
// handling requests. Since this is a known error, we should print a sensible
// error message to the end user.
func processGitalyError(statusErr error) error {
	if st, ok := grpcstatus.FromError(statusErr); ok {
		details := st.Details()
		for _, detail := range details {
			switch detail.(type) {
			case *pb.LimitError:
				return grpcstatus.Error(grpccodes.Unavailable, "GitLab is currently unable to handle this request due to load.")
			}
		}
	}

	return grpcstatus.Error(grpccodes.Unavailable, "The git server, Gitaly, is not available at this time. Please contact your administrator.")
}

// RunGitalyCommand provides a bootstrap for Gitaly commands executed
// through GitLab-Shell. It ensures that logging, tracing and other
// common concerns are configured before executing the `handler`.
func (gc *GitalyCommand) RunGitalyCommand(ctx context.Context, handler GitalyHandlerFunc) error {
	// We leave the connection open for future reuse
	conn, err := gc.getConn(ctx)
	if err != nil {
		log.ContextLogger(ctx).WithError(fmt.Errorf("RunGitalyCommand: %v", err)).Error("Failed to get connection to execute Git command")

		return err
	}

	childCtx := withOutgoingMetadata(ctx, gc.Response.Gitaly.Features)
	ctxlog := log.ContextLogger(childCtx)
	exitStatus, err := handler(childCtx, conn)

	if err != nil {
		ctxlog.WithError(err).WithFields(log.Fields{"exit_status": exitStatus}).Error("Failed to execute Git command")

		if grpcstatus.Code(err) == grpccodes.Unavailable {
			return processGitalyError(err)
		}
	}

	return err
}

// PrepareContext wraps a given context with a correlation ID and logs the command to
// be run.
func (gc *GitalyCommand) PrepareContext(ctx context.Context, repository *pb.Repository, env sshenv.Env) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	gc.LogExecution(ctx, repository, env)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	md.Append("key_id", strconv.Itoa(gc.Response.KeyId))
	md.Append("key_type", gc.Response.KeyType)
	md.Append("user_id", gc.Response.UserId)
	md.Append("username", gc.Response.Username)
	md.Append("remote_ip", env.RemoteAddr)
	ctx = metadata.NewOutgoingContext(ctx, md)

	return ctx, cancel
}

func (gc *GitalyCommand) LogExecution(ctx context.Context, repository *pb.Repository, env sshenv.Env) {
	fields := log.Fields{
		"command":         gc.Command.ServiceName,
		"gl_project_path": repository.GlProjectPath,
		"gl_repository":   repository.GlRepository,
		"user_id":         gc.Response.UserId,
		"username":        gc.Response.Username,
		"git_protocol":    env.GitProtocolVersion,
		"remote_ip":       env.RemoteAddr,
		"gl_key_type":     gc.Response.KeyType,
		"gl_key_id":       gc.Response.KeyId,
	}

	log.WithContextFields(ctx, fields).Info("executing git command")
}

func withOutgoingMetadata(ctx context.Context, features map[string]string) context.Context {
	md := metadata.New(nil)
	for k, v := range features {
		if !strings.HasPrefix(k, "gitaly-feature-") {
			continue
		}
		md.Append(k, v)
	}

	return metadata.NewOutgoingContext(ctx, md)
}

func (gc *GitalyCommand) getConn(ctx context.Context) (*grpc.ClientConn, error) {
	return gc.Config.GitalyClient.GetConnection(ctx, gc.Command)
}

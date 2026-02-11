// Package handler provides functionality for executing Gitaly commands
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	gitalyclient "gitlab.com/gitlab-org/gitaly/v18/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitaly"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
	"gitlab.com/gitlab-org/labkit/fields"

	pb "gitlab.com/gitlab-org/gitaly/v18/proto/go/gitalypb"
)

// GitalyHandlerFunc implementations are responsible for making
// an appropriate Gitaly call using the provided client and context
// and returning an error from the Gitaly call.
type GitalyHandlerFunc func(ctx context.Context, client *grpc.ClientConn) (int32, error)

// GitalyCommand provides functionality for executing Gitaly commands
type GitalyCommand struct {
	Config   *config.Config
	Response *accessverifier.Response
	Command  gitaly.Command
}

// NewGitalyCommand creates a new GitalyCommand instance
func NewGitalyCommand(cfg *config.Config, serviceName string, response *accessverifier.Response) *GitalyCommand {
	gc := gitaly.Command{
		CacheKey: gitaly.CacheKey{
			ServiceName: serviceName,
			Address:     response.Gitaly.Address,
			Token:       response.Gitaly.Token,
		},
		RetryPolicy: parseRetryConfig(response.RetryConfig),
	}

	return &GitalyCommand{Config: cfg, Response: response, Command: gc}
}

func parseRetryConfig(rawConfig json.RawMessage) *gitalyclient.RetryPolicy {
	if len(rawConfig) == 0 {
		return nil
	}

	var policy gitalyclient.RetryPolicy
	if err := protojson.Unmarshal(rawConfig, &policy); err != nil {
		slog.Error("failed to unmarshal retry policy", slog.String(fields.ErrorMessage, err.Error()))
		return nil
	}

	return &policy
}

// processGitalyError handles errors that come back from Gitaly that may be a
// LimitError. A LimitError is returned by Gitaly when it is at its limit in
// handling requests. Since this is a known error, we should print a sensible
// error message to the end user.
func processGitalyError(statusErr error) error {
	if st, ok := grpcstatus.FromError(statusErr); ok {
		details := st.Details()
		for _, detail := range details {
			if _, ok := detail.(*pb.LimitError); ok {
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
		slog.ErrorContext(ctx, "Failed to get connection to execute Git command", slog.String(fields.ErrorMessage, err.Error()))
		return err
	}

	childCtx := withOutgoingMetadata(ctx, gc.Response.Gitaly.Features)
	exitStatus, err := handler(childCtx, conn)

	if err != nil {
		slog.ErrorContext(ctx, "Failed to execute Git command", slog.String(fields.ErrorMessage, err.Error()), slog.Int("exit_status", int(exitStatus)))

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
	md.Append("key_id", strconv.Itoa(gc.Response.KeyID))
	md.Append("key_type", gc.Response.KeyType)
	md.Append("user_id", gc.Response.UserID)
	md.Append("username", gc.Response.Username)
	md.Append("remote_ip", env.RemoteAddr)
	ctx = metadata.NewOutgoingContext(ctx, md)

	return ctx, cancel
}

// LogExecution logs the execution of a Git command
func (gc *GitalyCommand) LogExecution(ctx context.Context, repository *pb.Repository, env sshenv.Env) {
	slog.InfoContext(ctx, "executing git command",
		slog.String("command", gc.Command.ServiceName),
		slog.String("gl_project_path", repository.GlProjectPath),
		slog.String("gl_repository", repository.GlRepository),
		slog.String("user_id", gc.Response.UserID),
		slog.String("username", gc.Response.Username),
		slog.String("git_protocol", env.GitProtocolVersion),
		slog.String("remote_ip", env.RemoteAddr),
		slog.String("gl_key_type", gc.Response.KeyType),
		slog.Int("gl_key_id", gc.Response.KeyID),
	)
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

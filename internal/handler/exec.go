package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"

	gitalyauth "gitlab.com/gitlab-org/gitaly/v14/auth"
	"gitlab.com/gitlab-org/gitaly/v14/client"
	pb "gitlab.com/gitlab-org/gitaly/v14/proto/go/gitalypb"
	"gitlab.com/gitlab-org/labkit/correlation"
	grpccorrelation "gitlab.com/gitlab-org/labkit/correlation/grpc"
	"gitlab.com/gitlab-org/labkit/log"
	grpctracing "gitlab.com/gitlab-org/labkit/tracing/grpc"
)

// GitalyHandlerFunc implementations are responsible for making
// an appropriate Gitaly call using the provided client and context
// and returning an error from the Gitaly call.
type GitalyHandlerFunc func(ctx context.Context, client *grpc.ClientConn) (int32, error)

type GitalyCommand struct {
	Config      *config.Config
	ServiceName string
	Address     string
	Token       string
	Features    map[string]string
}

// RunGitalyCommand provides a bootstrap for Gitaly commands executed
// through GitLab-Shell. It ensures that logging, tracing and other
// common concerns are configured before executing the `handler`.
func (gc *GitalyCommand) RunGitalyCommand(ctx context.Context, handler GitalyHandlerFunc) error {
	conn, err := getConn(ctx, gc)
	if err != nil {
		return err
	}
	defer conn.Close()

	childCtx := withOutgoingMetadata(ctx, gc.Features)
	_, err = handler(childCtx, conn)

	return err
}

// PrepareContext wraps a given context with a correlation ID and logs the command to
// be run.
func (gc *GitalyCommand) PrepareContext(ctx context.Context, repository *pb.Repository, response *accessverifier.Response, env sshenv.Env) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	gc.LogExecution(ctx, repository, response, env)

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	md.Append("key_id", strconv.Itoa(response.KeyId))
	md.Append("key_type", response.KeyType)
	md.Append("user_id", response.UserId)
	md.Append("username", response.Username)
	md.Append("remote_ip", env.RemoteAddr)
	ctx = metadata.NewOutgoingContext(ctx, md)

	return ctx, cancel
}

func (gc *GitalyCommand) LogExecution(ctx context.Context, repository *pb.Repository, response *accessverifier.Response, env sshenv.Env) {
	fields := log.Fields{
		"command":         gc.ServiceName,
		"gl_project_path": repository.GlProjectPath,
		"gl_repository":   repository.GlRepository,
		"user_id":         response.UserId,
		"username":        response.Username,
		"git_protocol":    env.GitProtocolVersion,
		"remote_ip":       env.RemoteAddr,
		"gl_key_type":     response.KeyType,
		"gl_key_id":       response.KeyId,
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

func getConn(ctx context.Context, gc *GitalyCommand) (*grpc.ClientConn, error) {
	if gc.Address == "" {
		return nil, fmt.Errorf("no gitaly_address given")
	}

	serviceName := correlation.ExtractClientNameFromContext(ctx)
	if serviceName == "" {
		serviceName = "gitlab-shell-unknown"

		log.WithContextFields(ctx, log.Fields{"service_name": serviceName}).Warn("No gRPC service name specified, defaulting to gitlab-shell-unknown")
	}

	serviceName = fmt.Sprintf("%s-%s", serviceName, gc.ServiceName)

	connOpts := client.DefaultDialOpts
	connOpts = append(
		connOpts,
		grpc.WithStreamInterceptor(
			grpc_middleware.ChainStreamClient(
				grpctracing.StreamClientTracingInterceptor(),
				grpc_prometheus.StreamClientInterceptor,
				grpccorrelation.StreamClientCorrelationInterceptor(
					grpccorrelation.WithClientName(serviceName),
				),
			),
		),

		grpc.WithUnaryInterceptor(
			grpc_middleware.ChainUnaryClient(
				grpctracing.UnaryClientTracingInterceptor(),
				grpc_prometheus.UnaryClientInterceptor,
				grpccorrelation.UnaryClientCorrelationInterceptor(
					grpccorrelation.WithClientName(serviceName),
				),
			),
		),
	)

	if gc.Token != "" {
		connOpts = append(connOpts,
			grpc.WithPerRPCCredentials(gitalyauth.RPCCredentialsV2(gc.Token)),
		)
	}

	return client.DialContext(ctx, gc.Address, connOpts)
}

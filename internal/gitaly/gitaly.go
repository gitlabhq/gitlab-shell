package gitaly

import (
	"context"
	"fmt"
	"sync"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"

	gitalyauth "gitlab.com/gitlab-org/gitaly/v15/auth"
	"gitlab.com/gitlab-org/gitaly/v15/client"
	gitalyclient "gitlab.com/gitlab-org/gitaly/v15/client"
	"gitlab.com/gitlab-org/labkit/correlation"
	grpccorrelation "gitlab.com/gitlab-org/labkit/correlation/grpc"
	"gitlab.com/gitlab-org/labkit/log"
	grpctracing "gitlab.com/gitlab-org/labkit/tracing/grpc"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

type Command struct {
	ServiceName string
	Address     string
	Token       string
}

type connectionsCache struct {
	sync.RWMutex

	connections map[Command]*grpc.ClientConn
}

type Client struct {
	SidechannelRegistry *gitalyclient.SidechannelRegistry

	cache connectionsCache
}

func (c *Client) InitSidechannelRegistry(ctx context.Context) {
	c.SidechannelRegistry = gitalyclient.NewSidechannelRegistry(log.ContextLogger(ctx))
}

func (c *Client) GetConnection(ctx context.Context, cmd Command) (*grpc.ClientConn, error) {
	c.cache.RLock()
	conn := c.cache.connections[cmd]
	c.cache.RUnlock()

	if conn != nil {
		return conn, nil
	}

	c.cache.Lock()
	defer c.cache.Unlock()

	if conn := c.cache.connections[cmd]; conn != nil {
		return conn, nil
	}

	conn, err := c.newConnection(ctx, cmd)
	if err != nil {
		return nil, err
	}

	if c.cache.connections == nil {
		c.cache.connections = make(map[Command]*grpc.ClientConn)
	}

	c.cache.connections[cmd] = conn

	return conn, nil
}

func (c *Client) newConnection(ctx context.Context, cmd Command) (conn *grpc.ClientConn, err error) {
	defer func() {
		label := "ok"
		if err != nil {
			label = "fail"
		}
		metrics.GitalyConnectionsTotal.WithLabelValues(label).Inc()
	}()

	if cmd.Address == "" {
		return nil, fmt.Errorf("no gitaly_address given")
	}

	serviceName := correlation.ExtractClientNameFromContext(ctx)
	if serviceName == "" {
		serviceName = "gitlab-shell-unknown"

		log.WithContextFields(ctx, log.Fields{"service_name": serviceName}).Warn("No gRPC service name specified, defaulting to gitlab-shell-unknown")
	}

	serviceName = fmt.Sprintf("%s-%s", serviceName, cmd.ServiceName)

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

		// In https://gitlab.com/groups/gitlab-org/-/epics/8971, we added DNS discovery support to Praefect. This was
		// done by making two changes:
		// - Configure client-side round-robin load-balancing in client dial options. We added that as a default option
		// inside gitaly client in gitaly client since v15.9.0
		// - Configure DNS resolving. Due to some technical limitations, we don't use gRPC's built-in DNS resolver.
		// Instead, we implement our own DNS resolver. This resolver is exposed via the following configuration.
		// Afterward, workhorse can detect and handle DNS discovery automatically. The user needs to setup and set
		// Gitaly address to something like "dns:gitaly.service.dc1.consul"
		gitalyclient.WithGitalyDNSResolver(gitalyclient.DefaultDNSResolverBuilderConfig()),
	)

	if cmd.Token != "" {
		connOpts = append(connOpts,
			grpc.WithPerRPCCredentials(gitalyauth.RPCCredentialsV2(cmd.Token)),
		)
	}

	return client.DialSidechannel(ctx, cmd.Address, c.SidechannelRegistry, connOpts)
}

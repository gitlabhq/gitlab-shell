package topology

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	"gitlab.com/gitlab-org/labkit/correlation"
	grpccorrelation "gitlab.com/gitlab-org/labkit/correlation/grpc"
	"gitlab.com/gitlab-org/labkit/log"
	grpctracing "gitlab.com/gitlab-org/labkit/tracing/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/metrics"
)

// ClassifyType constants mirror the proto enum values for convenience.
const (
	ClassifyTypeFirstCell     = pb.ClassifyType_FIRST_CELL
	ClassifyTypeSessionPrefix = pb.ClassifyType_SESSION_PREFIX
	ClassifyTypeCellID        = pb.ClassifyType_CELL_ID
)

// Metric status labels
const (
	metricsStatusOK   = "ok"
	metricsStatusFail = "fail"
)

// Client provides a gRPC client for the Topology Service.
// It handles connection management with lazy initialization and
// supports TLS/mTLS for secure connections.
type Client struct {
	config *Config

	mu     sync.Mutex
	conn   *grpc.ClientConn
	client pb.ClassifyServiceClient
}

// NewClient creates a new Topology Service client from the given configuration.
// Returns nil if the topology service is disabled in the configuration.
// The client uses lazy initialization - the actual gRPC connection is
// established on the first call to Classify.
// The configuration is copied to avoid mutating the original.
func NewClient(cfg *Config) *Client {
	if !cfg.Enabled {
		return nil
	}

	// Copy config to avoid mutating the original
	configCopy := *cfg

	// Apply defaults
	if configCopy.Timeout == 0 {
		configCopy.Timeout = DefaultTimeout
	}
	if configCopy.ClassifyType == "" {
		configCopy.ClassifyType = "first_cell"
	}

	return &Client{
		config: &configCopy,
	}
}

// Classify queries the Topology Service to determine which cell should handle
// a request for the given value. The value interpretation depends on the
// configured ClassifyType (e.g., project path, session prefix, cell ID).
func (c *Client) Classify(ctx context.Context, value string) (*pb.ClassifyResponse, error) {
	start := time.Now()
	var status string

	defer func() {
		metrics.TopologyRequestsTotal.WithLabelValues(status).Inc()
		metrics.TopologyRequestDurationSeconds.Observe(time.Since(start).Seconds())
	}()

	client, err := c.getClient(ctx)
	if err != nil {
		status = metricsStatusFail
		return nil, fmt.Errorf("failed to get topology client: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	req := &pb.ClassifyRequest{
		Type:  parseClassifyType(c.config.ClassifyType),
		Value: value,
	}

	resp, err := client.Classify(ctx, req)
	if err != nil {
		status = metricsStatusFail
		return nil, err
	}

	status = metricsStatusOK
	return resp, nil
}

// Close closes the gRPC connection to the Topology Service.
// It is safe to call Close multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.conn = nil
	c.client = nil
	return err
}

// getClient returns the ClassifyService client, establishing a connection if needed.
// This implements lazy initialization - the connection is only created on first use.
func (c *Client) getClient(ctx context.Context) (pb.ClassifyServiceClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		return c.client, nil
	}

	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}

	c.conn = conn
	c.client = pb.NewClassifyServiceClient(conn)
	return c.client, nil
}

// dial establishes a gRPC connection to the Topology Service.
func (c *Client) dial(ctx context.Context) (conn *grpc.ClientConn, err error) {
	defer func() {
		status := metricsStatusOK
		if err != nil {
			status = metricsStatusFail
		}
		metrics.TopologyConnectionsTotal.WithLabelValues(status).Inc()
	}()

	serviceName := correlation.ExtractClientNameFromContext(ctx)
	if serviceName == "" {
		serviceName = "gitlab-shell-unknown"

		log.WithContextFields(ctx, log.Fields{"service_name": serviceName}).Warn("No gRPC service name specified, defaulting to gitlab-shell-unknown")
	}
	serviceName = fmt.Sprintf("%s-%s", serviceName, "topology")

	opts := []grpc.DialOption{
		grpc.WithChainStreamInterceptor(
			grpctracing.StreamClientTracingInterceptor(),
			grpc_prometheus.StreamClientInterceptor,
			grpccorrelation.StreamClientCorrelationInterceptor(
				grpccorrelation.WithClientName(serviceName),
			),
		),
		grpc.WithChainUnaryInterceptor(
			grpctracing.UnaryClientTracingInterceptor(),
			grpc_prometheus.UnaryClientInterceptor,
			grpccorrelation.UnaryClientCorrelationInterceptor(
				grpccorrelation.WithClientName(serviceName),
			),
		),
	}

	creds, err := buildTLSCredentials(c.config)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS credentials: %w", err)
	}

	if creds != nil {
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return grpc.NewClient(c.config.Address, opts...)
}

// buildTLSCredentials creates gRPC transport credentials based on the TLS configuration.
// Returns nil if TLS is not enabled.
func buildTLSCredentials(cfg *Config) (credentials.TransportCredentials, error) {
	if !cfg.TLS.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.TLS.ServerName,
		InsecureSkipVerify: cfg.TLS.InsecureSkipVerify, //nolint:gosec // Intentionally configurable for development/testing
	}

	// Load CA certificate if specified
	if cfg.TLS.CAFile != "" {
		caCert, err := os.ReadFile(cfg.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}

		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caPool
	}

	// Load client certificate and key for mTLS
	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// parseClassifyType converts a string classify type to the proto enum value.
// Returns FIRST_CELL as the default for unrecognized values.
func parseClassifyType(classifyType string) pb.ClassifyType {
	switch classifyType {
	case "first_cell":
		return pb.ClassifyType_FIRST_CELL
	case "session_prefix":
		return pb.ClassifyType_SESSION_PREFIX
	case "cell_id":
		return pb.ClassifyType_CELL_ID
	default:
		return pb.ClassifyType_FIRST_CELL
	}
}

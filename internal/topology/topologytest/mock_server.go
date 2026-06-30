// Package topologytest provides shared test helpers for the Topology Service
// mock gRPC server. It is used by tests in the topology and accessverifier
// packages to avoid duplicating the mock ClassifyService implementation.
package topologytest

import (
	"context"
	"net"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/cells/topology-service/clients/go/proto"
	"google.golang.org/grpc"
)

// MockClassifyServer implements the ClassifyService for testing.
// Configure Response and/or Err before issuing requests. When both are nil
// a default PROXY response pointing at "cell-1.gitlab.com:443" is returned.
type MockClassifyServer struct {
	pb.UnimplementedClassifyServiceServer

	// Response is the canned response returned by Classify.
	Response *pb.ClassifyResponse
	// Err is returned instead of a response when non-nil.
	Err error
	// LastRequest is the most recent ClassifyRequest received by the server.
	LastRequest *pb.ClassifyRequest
	// CallCount is incremented on each Classify call.
	CallCount int
	// ErrUntilAttempt, when set to N, causes the server to return Err for
	// calls 1..N-1 and then succeed from call N onwards.
	ErrUntilAttempt int
}

// Classify implements the ClassifyService RPC.
func (m *MockClassifyServer) Classify(_ context.Context, req *pb.ClassifyRequest) (*pb.ClassifyResponse, error) {
	m.CallCount++
	m.LastRequest = req

	if m.Err != nil && (m.ErrUntilAttempt == 0 || m.CallCount < m.ErrUntilAttempt) {
		return nil, m.Err
	}

	if m.Response != nil {
		return m.Response, nil
	}
	return &pb.ClassifyResponse{
		Action: pb.ClassifyAction_PROXY,
		Proxy:  &pb.ProxyInfo{Address: "cell-1.gitlab.com:443"},
	}, nil
}

// StartMockServer starts a gRPC server with the given mock and returns the
// listener address and a stop function. The caller must defer the stop
// function to shut down the server.
func StartMockServer(t *testing.T, mock *MockClassifyServer) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	pb.RegisterClassifyServiceServer(server, mock)

	go func() { _ = server.Serve(lis) }()

	return lis.Addr().String(), server.Stop
}

// CellAddressOverride captures the inputs for testing Topology Service port
// override behavior. TopologyAddress is the cell host joined with a bogus port
// (use it for the PROXY response Address); RealPort is the cell server's actual
// listening port (configure it as CellEndpoint.Port). The request only reaches
// the cell server if the resolver replaces the bogus port with RealPort.
type CellAddressOverride struct {
	TopologyAddress string
	RealPort        int
}

// CellAddressWithBogusPort returns a CellAddressOverride built from the cell
// server's host: TopologyAddress uses the given bogus port, RealPort is the
// server's real port.
func CellAddressWithBogusPort(t *testing.T, cellServer *httptest.Server, bogusPort int) CellAddressOverride {
	t.Helper()
	addr := cellServer.Listener.Addr().(*net.TCPAddr)
	host := addr.IP.String()
	return CellAddressOverride{
		TopologyAddress: net.JoinHostPort(host, strconv.Itoa(bogusPort)),
		RealPort:        addr.Port,
	}
}

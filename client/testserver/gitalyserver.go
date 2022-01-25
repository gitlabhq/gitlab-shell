package testserver

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly/v14/client"
	pb "gitlab.com/gitlab-org/gitaly/v14/proto/go/gitalypb"
	"gitlab.com/gitlab-org/labkit/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type TestGitalyServer struct {
	ReceivedMD metadata.MD
	pb.UnimplementedSSHServiceServer
}

func (s *TestGitalyServer) SSHReceivePack(stream pb.SSHService_SSHReceivePackServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	s.ReceivedMD, _ = metadata.FromIncomingContext(stream.Context())

	response := []byte("ReceivePack: " + req.GlId + " " + req.Repository.GlRepository)
	stream.Send(&pb.SSHReceivePackResponse{Stdout: response})

	return nil
}

func (s *TestGitalyServer) SSHUploadPack(stream pb.SSHService_SSHUploadPackServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	s.ReceivedMD, _ = metadata.FromIncomingContext(stream.Context())

	response := []byte("UploadPack: " + req.Repository.GlRepository)
	stream.Send(&pb.SSHUploadPackResponse{Stdout: response})

	return nil
}

func (s *TestGitalyServer) SSHUploadPackWithSidechannel(ctx context.Context, req *pb.SSHUploadPackWithSidechannelRequest) (*pb.SSHUploadPackWithSidechannelResponse, error) {
	conn, err := client.OpenServerSidechannel(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	s.ReceivedMD, _ = metadata.FromIncomingContext(ctx)

	response := []byte("SSHUploadPackWithSidechannel: " + req.Repository.GlRepository)
	if _, err := fmt.Fprintf(conn, "%04x\x01%s", len(response)+5, response); err != nil {
		return nil, err
	}
	if err := conn.Close(); err != nil {
		return nil, err
	}

	return &pb.SSHUploadPackWithSidechannelResponse{}, nil
}

func (s *TestGitalyServer) SSHUploadArchive(stream pb.SSHService_SSHUploadArchiveServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	s.ReceivedMD, _ = metadata.FromIncomingContext(stream.Context())

	response := []byte("UploadArchive: " + req.Repository.GlRepository)
	stream.Send(&pb.SSHUploadArchiveResponse{Stdout: response})

	return nil
}

func StartGitalyServer(t *testing.T) (string, *TestGitalyServer) {
	t.Helper()

	tempDir, _ := os.MkdirTemp("", "gitlab-shell-test-api")
	gitalySocketPath := path.Join(tempDir, "gitaly.sock")
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	err := os.MkdirAll(filepath.Dir(gitalySocketPath), 0700)
	require.NoError(t, err)

	server := grpc.NewServer(
		client.SidechannelServer(log.ContextLogger(context.Background()), insecure.NewCredentials()),
	)

	listener, err := net.Listen("unix", gitalySocketPath)
	require.NoError(t, err)

	testServer := TestGitalyServer{}
	pb.RegisterSSHServiceServer(server, &testServer)

	go server.Serve(listener)
	t.Cleanup(func() { server.Stop() })

	gitalySocketUrl := "unix:" + gitalySocketPath

	return gitalySocketUrl, &testServer
}

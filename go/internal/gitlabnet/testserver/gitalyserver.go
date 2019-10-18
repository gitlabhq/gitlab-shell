package testserver

import (
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

type TestGitalyServer struct{ ReceivedMD metadata.MD }

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

func StartGitalyServer(t *testing.T) (string, *TestGitalyServer, func()) {
	tempDir, _ := ioutil.TempDir("", "gitlab-shell-test-api")
	gitalySocketPath := path.Join(tempDir, "gitaly.sock")

	err := os.MkdirAll(filepath.Dir(gitalySocketPath), 0700)
	require.NoError(t, err)

	server := grpc.NewServer()

	listener, err := net.Listen("unix", gitalySocketPath)
	require.NoError(t, err)

	testServer := TestGitalyServer{}
	pb.RegisterSSHServiceServer(server, &testServer)

	go server.Serve(listener)

	gitalySocketUrl := "unix:" + gitalySocketPath
	cleanup := func() {
		server.Stop()
		os.RemoveAll(tempDir)
	}

	return gitalySocketUrl, &testServer, cleanup
}

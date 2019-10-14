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

	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

type testGitalyServer struct{}

func (s *testGitalyServer) SSHReceivePack(stream pb.SSHService_SSHReceivePackServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	response := []byte("ReceivePack: " + req.GlId + " " + req.Repository.GlRepository)
	stream.Send(&pb.SSHReceivePackResponse{Stdout: response})

	return nil
}

func (s *testGitalyServer) SSHUploadPack(stream pb.SSHService_SSHUploadPackServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	response := []byte("UploadPack: " + req.Repository.GlRepository)
	stream.Send(&pb.SSHUploadPackResponse{Stdout: response})

	return nil
}

func (s *testGitalyServer) SSHUploadArchive(stream pb.SSHService_SSHUploadArchiveServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	response := []byte("UploadArchive: " + req.Repository.GlRepository)
	stream.Send(&pb.SSHUploadArchiveResponse{Stdout: response})

	return nil
}

func StartGitalyServer(t *testing.T) (string, func()) {
	tempDir, _ := ioutil.TempDir("", "gitlab-shell-test-api")
	gitalySocketPath := path.Join(tempDir, "gitaly.sock")

	err := os.MkdirAll(filepath.Dir(gitalySocketPath), 0700)
	require.NoError(t, err)

	server := grpc.NewServer()

	listener, err := net.Listen("unix", gitalySocketPath)
	require.NoError(t, err)

	pb.RegisterSSHServiceServer(server, &testGitalyServer{})

	go server.Serve(listener)

	gitalySocketUrl := "unix:" + gitalySocketPath
	cleanup := func() {
		server.Stop()
		os.RemoveAll(tempDir)
	}

	return gitalySocketUrl, cleanup
}

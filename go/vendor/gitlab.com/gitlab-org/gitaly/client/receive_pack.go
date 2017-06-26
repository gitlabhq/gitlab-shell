package client

import (
	"fmt"
	"io"

	"google.golang.org/grpc"

	"golang.org/x/net/context"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
	pbhelper "gitlab.com/gitlab-org/gitaly-proto/go/helper"
)

// ReceivePack proxies an SSH git-receive-pack (git push) session to Gitaly
func ReceivePack(ctx context.Context, conn *grpc.ClientConn, stdin io.Reader, stdout, stderr io.Writer, req *pb.SSHReceivePackRequest) (int32, error) {
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	ssh := pb.NewSSHClient(conn)
	stream, err := ssh.SSHReceivePack(ctx2)
	if err != nil {
		return 0, err
	}

	if err = stream.Send(req); err != nil {
		return 0, err
	}

	inWriter := pbhelper.NewSendWriter(func(p []byte) error {
		return stream.Send(&pb.SSHReceivePackRequest{Stdin: p})
	})

	errC := make(chan error, 1)

	go func() {
		_, errRecv := io.Copy(inWriter, stdin)
		stream.CloseSend()
		errC <- errRecv
	}()

	exitStatus, errRecv := recvStdoutStderrStream(func() (stdoutStderrResponse, error) {
		return stream.Recv()
	}, stdout, stderr)

	if errRecv != nil {
		return exitStatus, errRecv
	}

	select {
	case errSend := <-errC:
		if errSend != nil {
			// This should not happen
			errSend = fmt.Errorf("stdin send error: %v", errSend)
		}
		return exitStatus, errSend
	default:
		return exitStatus, nil
	}
}

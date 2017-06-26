package client

import (
	"io"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
)

type stdoutStderrResponse interface {
	GetExitStatus() *pb.ExitStatus
	GetStderr() []byte
	GetStdout() []byte
}

func recvStdoutStderrStream(recv func() (stdoutStderrResponse, error), stdout, stderr io.Writer) (int32, error) {
	var (
		exitStatus int32
		err        error
		resp       stdoutStderrResponse
	)
	for {
		resp, err = recv()
		if err != nil {
			break
		}
		if resp.GetExitStatus() != nil {
			exitStatus = resp.GetExitStatus().GetValue()
		}

		if len(resp.GetStderr()) > 0 {
			if _, errWrite := stderr.Write(resp.GetStderr()); errWrite != nil {
				return exitStatus, errWrite
			}
		}

		if len(resp.GetStdout()) > 0 {
			if _, errWrite := stdout.Write(resp.GetStdout()); errWrite != nil {
				return exitStatus, errWrite
			}
		}
	}
	if err == io.EOF {
		err = nil
	}
	return exitStatus, err
}

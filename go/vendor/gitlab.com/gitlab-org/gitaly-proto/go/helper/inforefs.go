package helper

import (
	"io"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
)

type InfoRefsClient interface {
	Recv() (*pb.InfoRefsResponse, error)
}

type InfoRefsClientWriterTo struct {
	InfoRefsClient
}

func (clientReader *InfoRefsClientWriterTo) WriteTo(w io.Writer) (total int64, err error) {
	for {
		response, err := clientReader.Recv()
		if err == io.EOF {
			return total, nil
		} else if err != nil {
			return total, err
		}

		n, err := w.Write(response.GetData())
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
}

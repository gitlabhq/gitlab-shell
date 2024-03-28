package cells

import (
	"google.golang.org/grpc"
)

type Client struct {
	conn *grpc.ClientConn
}

func NewClient(url string) (*Client, error) {
	conn, err := grpc.Dial(url, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return &Client{conn: conn}, nil
}

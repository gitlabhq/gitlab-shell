package cells

import (
	"context"
	"strings"

	pb "gitlab.com/gitlab-org/cells/global-service-poc/gen/go/proto"
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

func (c *Client) Classify(ctx context.Context, repo string) (*pb.CellInfo, error) {
	req := &pb.ClassifyRequest{
		Match: pb.ClassifyMatch_Route,
		Value: strings.Split(strings.TrimPrefix(repo, "/"), "/")[0],
	}

	client := pb.NewClassifyServiceClient(c.conn)

	response, err := client.Classify(ctx, req)
	if err != nil {
		return nil, err
	}
	cell := response.CellInfo

	return cell, nil
}

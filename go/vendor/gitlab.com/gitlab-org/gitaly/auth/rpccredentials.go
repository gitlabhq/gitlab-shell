package gitalyauth

import (
	"encoding/base64"

	"golang.org/x/net/context"
	"google.golang.org/grpc/credentials"
)

// RPCCredentials can be used with grpc.WithPerRPCCredentials to create a
// grpc.DialOption that inserts the supplied token for authentication
// with a Gitaly server.
func RPCCredentials(token string) credentials.PerRPCCredentials {
	return &rpcCredentials{token: base64.StdEncoding.EncodeToString([]byte(token))}
}

type rpcCredentials struct {
	token string
}

func (*rpcCredentials) RequireTransportSecurity() bool { return false }

func (rc *rpcCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + rc.token}, nil
}

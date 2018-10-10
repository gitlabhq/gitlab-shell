package gitalyauth

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

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

// RPCCredentialsV2 can be used with grpc.WithPerRPCCredentials to create a
// grpc.DialOption that inserts an HMAC token with the current timestamp
// for authentication with a Gitaly server.
func RPCCredentialsV2(token string) credentials.PerRPCCredentials {
	return &rpcCredentialsV2{token: token}
}

type rpcCredentialsV2 struct {
	token string
}

func (*rpcCredentialsV2) RequireTransportSecurity() bool { return false }

func (rc *rpcCredentialsV2) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + rc.hmacToken()}, nil
}

func (rc *rpcCredentialsV2) hmacToken() string {
	return hmacToken("v2", []byte(rc.token), time.Now())
}

func hmacToken(version string, secret []byte, timestamp time.Time) string {
	intTime := timestamp.Unix()
	signedTimestamp := hmacSign(secret, strconv.FormatInt(intTime, 10))

	return fmt.Sprintf("%s.%x.%d", version, signedTimestamp, intTime)
}

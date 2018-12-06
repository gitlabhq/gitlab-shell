package client

import (
	"google.golang.org/grpc/credentials"

	"net/url"

	"google.golang.org/grpc"
)

// DefaultDialOpts hold the default DialOptions for connection to Gitaly over UNIX-socket
var DefaultDialOpts = []grpc.DialOption{}

// Dial gitaly
func Dial(rawAddress string, connOpts []grpc.DialOption) (*grpc.ClientConn, error) {
	canonicalAddress, err := parseAddress(rawAddress)
	if err != nil {
		return nil, err
	}

	if isTLS(rawAddress) {
		certPool, err := systemCertPool()
		if err != nil {
			return nil, err
		}

		creds := credentials.NewClientTLSFromCert(certPool, "")
		connOpts = append(connOpts, grpc.WithTransportCredentials(creds))
	} else {
		connOpts = append(connOpts, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(canonicalAddress, connOpts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func isTLS(rawAddress string) bool {
	u, err := url.Parse(rawAddress)
	return err == nil && u.Scheme == "tls"
}

package client

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc"
)

// DefaultDialOpts hold the default DialOptions for connection to Gitaly over UNIX-socket
var DefaultDialOpts = []grpc.DialOption{
	grpc.WithInsecure(),
}

// Dial gitaly
func Dial(rawAddress string, connOpts []grpc.DialOption) (*grpc.ClientConn, error) {
	network, addr, err := parseAddress(rawAddress)
	if err != nil {
		return nil, err
	}

	connOpts = append(connOpts,
		grpc.WithDialer(func(a string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout(network, a, timeout)
		}))
	conn, err := grpc.Dial(addr, connOpts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func parseAddress(rawAddress string) (network, addr string, err error) {
	// Parsing unix:// URL's with url.Parse does not give the result we want
	// so we do it manually.
	for _, prefix := range []string{"unix://", "unix:"} {
		if strings.HasPrefix(rawAddress, prefix) {
			return "unix", strings.TrimPrefix(rawAddress, prefix), nil
		}
	}

	u, err := url.Parse(rawAddress)
	if err != nil {
		return "", "", err
	}

	if u.Scheme != "tcp" {
		return "", "", fmt.Errorf("unknown scheme: %q", rawAddress)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("network tcp requires host: %q", rawAddress)
	}
	if u.Path != "" {
		return "", "", fmt.Errorf("network tcp should have no path: %q", rawAddress)
	}
	return "tcp", u.Host, nil
}

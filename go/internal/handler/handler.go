package handler

import (
	"crypto/x509"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"gitlab.com/gitlab-org/gitaly/auth"
	"gitlab.com/gitlab-org/gitaly/client"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/go/internal/logger"
)

func Prepare() error {
	cfg, err := config.New()
	if err != nil {
		return err
	}

	if err := logger.Configure(cfg); err != nil {
		return err
	}

	// Use a working directory that won't get removed or unmounted.
	if err := os.Chdir("/"); err != nil {
		return err
	}

	return nil
}

func transFormTls(gitalyAddress string) (string, bool) {
	if !strings.HasPrefix(gitalyAddress, "tls://") {
		return gitalyAddress, false
	}

	return strings.Replace(gitalyAddress, "tls://", "tcp://", 1), true
}

func execCommand(command string, args ...string) error {
	binPath, err := exec.LookPath(command)
	if err != nil {
		return err
	}

	args = append([]string{binPath}, args...)
	return syscall.Exec(binPath, args, os.Environ())
}

func dialOpts(tls bool) []grpc.DialOption {
	connOpts := client.DefaultDialOpts
	if token := os.Getenv("GITALY_TOKEN"); token != "" {
		connOpts = append(client.DefaultDialOpts, grpc.WithPerRPCCredentials(gitalyauth.RPCCredentialsV2(token)))
	}

	if tls {
		certPool, err := x509.SystemCertPool()
		if err == nil {
			creds := credentials.NewClientTLSFromCert(certPool, "")
			connOpts = append(connOpts, grpc.WithTransportCredentials(creds))
		}
	}

	return connOpts
}

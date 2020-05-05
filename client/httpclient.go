package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const (
	socketBaseUrl             = "http://unix"
	unixSocketProtocol        = "http+unix://"
	httpProtocol              = "http://"
	httpsProtocol             = "https://"
	defaultReadTimeoutSeconds = 300
)

type HttpClient struct {
	*http.Client
	Host string
}

func NewHTTPClient(gitlabURL, caFile, caPath string, selfSignedCert bool, readTimeoutSeconds uint64) *HttpClient {

	var transport *http.Transport
	var host string
	if strings.HasPrefix(gitlabURL, unixSocketProtocol) {
		transport, host = buildSocketTransport(gitlabURL)
	} else if strings.HasPrefix(gitlabURL, httpProtocol) {
		transport, host = buildHttpTransport(gitlabURL)
	} else if strings.HasPrefix(gitlabURL, httpsProtocol) {
		transport, host = buildHttpsTransport(caFile, caPath, selfSignedCert, gitlabURL)
	} else {
		return nil
	}

	c := &http.Client{
		Transport: transport,
		Timeout:   readTimeout(readTimeoutSeconds),
	}

	client := &HttpClient{Client: c, Host: host}

	return client
}

func buildSocketTransport(gitlabURL string) (*http.Transport, string) {
	socketPath := strings.TrimPrefix(gitlabURL, unixSocketProtocol)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}

	return transport, socketBaseUrl
}

func buildHttpsTransport(caFile, caPath string, selfSignedCert bool, gitlabURL string) (*http.Transport, string) {
	certPool, err := x509.SystemCertPool()

	if err != nil {
		certPool = x509.NewCertPool()
	}

	if caFile != "" {
		addCertToPool(certPool, caFile)
	}

	if caPath != "" {
		fis, _ := ioutil.ReadDir(caPath)
		for _, fi := range fis {
			if fi.IsDir() {
				continue
			}

			addCertToPool(certPool, filepath.Join(caPath, fi.Name()))
		}
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:            certPool,
			InsecureSkipVerify: selfSignedCert,
		},
	}

	return transport, gitlabURL
}

func addCertToPool(certPool *x509.CertPool, fileName string) {
	cert, err := ioutil.ReadFile(fileName)
	if err == nil {
		certPool.AppendCertsFromPEM(cert)
	}
}

func buildHttpTransport(gitlabURL string) (*http.Transport, string) {
	return &http.Transport{}, gitlabURL
}

func readTimeout(timeoutSeconds uint64) time.Duration {
	if timeoutSeconds == 0 {
		timeoutSeconds = defaultReadTimeoutSeconds
	}

	return time.Duration(timeoutSeconds) * time.Second
}

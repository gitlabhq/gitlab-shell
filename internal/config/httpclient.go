package config

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
	HttpClient *http.Client
	Host       string
}

func (c *Config) GetHttpClient() *HttpClient {
	if c.HttpClient != nil {
		return c.HttpClient
	}

	var transport *http.Transport
	var host string
	if strings.HasPrefix(c.GitlabUrl, unixSocketProtocol) {
		transport, host = c.buildSocketTransport()
	} else if strings.HasPrefix(c.GitlabUrl, httpProtocol) {
		transport, host = c.buildHttpTransport()
	} else if strings.HasPrefix(c.GitlabUrl, httpsProtocol) {
		transport, host = c.buildHttpsTransport()
	} else {
		return nil
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   c.readTimeout(),
	}

	client := &HttpClient{HttpClient: httpClient, Host: host}

	c.HttpClient = client

	return client
}

func (c *Config) buildSocketTransport() (*http.Transport, string) {
	socketPath := strings.TrimPrefix(c.GitlabUrl, unixSocketProtocol)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}

	return transport, socketBaseUrl
}

func (c *Config) buildHttpsTransport() (*http.Transport, string) {
	certPool, err := x509.SystemCertPool()

	if err != nil {
		certPool = x509.NewCertPool()
	}

	caFile := c.HttpSettings.CaFile
	if caFile != "" {
		addCertToPool(certPool, caFile)
	}

	caPath := c.HttpSettings.CaPath
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
			InsecureSkipVerify: c.HttpSettings.SelfSignedCert,
		},
	}

	return transport, c.GitlabUrl
}

func addCertToPool(certPool *x509.CertPool, fileName string) {
	cert, err := ioutil.ReadFile(fileName)
	if err == nil {
		certPool.AppendCertsFromPEM(cert)
	}
}

func (c *Config) buildHttpTransport() (*http.Transport, string) {
	return &http.Transport{}, c.GitlabUrl
}

func (c *Config) readTimeout() time.Duration {
	timeoutSeconds := c.HttpSettings.ReadTimeoutSeconds

	if timeoutSeconds == 0 {
		timeoutSeconds = defaultReadTimeoutSeconds
	}

	return time.Duration(timeoutSeconds) * time.Second
}

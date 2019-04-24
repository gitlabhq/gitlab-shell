package config

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	socketBaseUrl             = "http://unix"
	UnixSocketProtocol        = "http+unix://"
	HttpProtocol              = "http://"
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
	if strings.HasPrefix(c.GitlabUrl, UnixSocketProtocol) {
		transport, host = c.buildSocketTransport()
	} else if strings.HasPrefix(c.GitlabUrl, HttpProtocol) {
		transport, host = c.buildHttpTransport()
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
	socketPath := strings.TrimPrefix(c.GitlabUrl, UnixSocketProtocol)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}

	return transport, socketBaseUrl
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

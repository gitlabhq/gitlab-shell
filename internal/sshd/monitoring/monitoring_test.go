package monitoring

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshd"
)

const (
	certFile = "testdata/localhost.crt"
	keyFile  = "testdata/localhost.key"
)

func TestRun(t *testing.T) {
	server := WebServer{
		ListenerConfigs: []config.ListenerConfig{
			{
				Addr: "127.0.0.1:0",
			},
			{
				Addr: "127.0.0.1:0",
				Tls: &config.TlsConfig{
					Certificate: certFile,
					Key:         keyFile,
				},
			},
			{
				Addr: "",
			},
		},
	}

	sshdServer := &sshd.Server{Config: &config.Config{Server: config.DefaultServerConfig}}
	require.NoError(t, server.Start("2021-02-16T09:28:07+01:00", "(unknown)", sshdServer))

	require.Len(t, server.listeners, 2)

	for url, client := range buildClients(t, server.listeners) {
		resp, err := client.Get(url)
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode, "get: "+url)
	}
}

func buildHttpsClient(t *testing.T) *http.Client {
	t.Helper()

	client := &http.Client{}
	certpool := x509.NewCertPool()

	tlsCertificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	require.NoError(t, err)

	certificate, err := x509.ParseCertificate(tlsCertificate.Certificate[0])
	require.NoError(t, err)

	certpool.AddCert(certificate)
	client.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: certpool,
		},
	}

	return client
}

func buildClients(t *testing.T, listeners []net.Listener) map[string]*http.Client {
	httpListener, httpsListener := listeners[0], listeners[1]
	httpsClient := buildHttpsClient(t)

	clients := map[string]*http.Client{}

	clients["http://"+httpListener.Addr().String()+"/metrics"] = http.DefaultClient
	clients["http://"+httpListener.Addr().String()+"/debug/pprof"] = http.DefaultClient
	clients["http://"+httpListener.Addr().String()+"/health"] = http.DefaultClient
	clients["https://"+httpsListener.Addr().String()+"/metrics"] = httpsClient
	clients["https://"+httpsListener.Addr().String()+"/debug/pprof"] = httpsClient
	clients["https://"+httpsListener.Addr().String()+"/health"] = httpsClient

	return clients
}

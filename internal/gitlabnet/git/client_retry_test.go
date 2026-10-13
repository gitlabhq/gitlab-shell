package git

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
)

const requestBody = "0032want 0a53e9ddeaddad63ad106860237bbf53411d11a7\n0000"

// TestSSHUploadPackRetriesAfterGoAway reproduces the production failure: the
// pooled HTTP/2 connection to the edge is closed with GOAWAY after the request
// was sent but before it was processed. With a replayable body the transport
// resends the request on a new connection; with a pipe it cannot.
func TestSSHUploadPackRetriesAfterGoAway(t *testing.T) {
	t.Run("replayable body is resent on a new connection", func(t *testing.T) {
		server, state := startGoAwayServer(t)
		useTestTransport(t, server)

		body := NewReplayBody(DefaultReplayLimit)
		_, err := io.WriteString(body, requestBody)
		require.NoError(t, err)
		require.NoError(t, body.Close())

		gitClient := &Client{URL: server.URL}
		response, err := gitClient.SSHUploadPack(context.Background(), body.Reader())
		require.NoError(t, err)
		defer response.Body.Close()

		result, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "ssh-upload-pack: "+requestBody, string(result))

		require.EqualValues(t, 1, state.handled.Load(), "the request is handled exactly once")
		require.GreaterOrEqual(t, state.accepted.Load(), int32(2), "a second connection was dialed after GOAWAY")
	})

	t.Run("pipe body cannot be resent", func(t *testing.T) {
		server, state := startGoAwayServer(t)
		useTestTransport(t, server)

		pipeReader, pipeWriter := io.Pipe()
		go func() {
			_, _ = io.WriteString(pipeWriter, requestBody)
			_ = pipeWriter.Close()
		}()

		gitClient := &Client{URL: server.URL}
		response, err := gitClient.SSHUploadPack(context.Background(), pipeReader) //nolint:bodyclose // no response on error
		require.Nil(t, response)
		require.EqualError(t, err, repoUnavailableErrMsg)
		require.EqualValues(t, 0, state.handled.Load(), "the request never reached a handler")
	})
}

type goAwayServerState struct {
	accepted atomic.Int32
	handled  atomic.Int32
}

// startGoAwayServer starts an HTTPS server with HTTP/2 whose first TCP
// connection is served by hand: it completes the TLS and HTTP/2 handshakes,
// waits for the client's HEADERS and first DATA frame, then sends GOAWAY with
// last-stream-id 0 and closes. Later connections reach the real handler.
func startGoAwayServer(t *testing.T) (*httptest.Server, *goAwayServerState) {
	t.Helper()
	state := &goAwayServerState{}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.handled.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("ssh-upload-pack: " + string(body)))
	}))
	server.EnableHTTP2 = true
	listener := &goAwayListener{Listener: server.Listener, state: state, tlsConfig: func() *tls.Config { return server.TLS }}
	server.Listener = listener
	server.StartTLS()
	t.Cleanup(server.Close)

	return server, state
}

// useTestTransport points the package HTTP client at the test server's CA for
// the duration of the test, with HTTP/2 enabled like the production transport.
func useTestTransport(t *testing.T, server *httptest.Server) {
	t.Helper()

	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	transport.ForceAttemptHTTP2 = true

	previous := httpClient
	httpClient = &http.Client{Transport: client.NewTransport(transport), CheckRedirect: checkRedirect}
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		httpClient = previous
	})
}

type goAwayListener struct {
	net.Listener
	state     *goAwayServerState
	tlsConfig func() *tls.Config
	first     sync.Once
}

func (l *goAwayListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		l.state.accepted.Add(1)

		hijacked := false
		l.first.Do(func() { hijacked = true })
		if !hijacked {
			return conn, nil
		}
		go sendGoAwayAfterRequest(conn, l.tlsConfig())
	}
}

// HTTP/2 frame types and the client connection preface (RFC 9113).
const (
	frameData     = 0x0
	frameHeaders  = 0x1
	frameSettings = 0x4
	frameGoAway   = 0x7
	flagAck       = 0x1
	clientPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
)

func sendGoAwayAfterRequest(raw net.Conn, cfg *tls.Config) {
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(5 * time.Second))

	conn := tls.Server(raw, cfg)
	if err := conn.Handshake(); err != nil || conn.ConnectionState().NegotiatedProtocol != "h2" {
		return
	}

	preface := make([]byte, len(clientPreface))
	if _, err := io.ReadFull(conn, preface); err != nil || string(preface) != clientPreface {
		return
	}
	// Empty server SETTINGS.
	if _, err := conn.Write(frameHeader(0, frameSettings, 0, 0)); err != nil {
		return
	}

	sawHeaders, sawData := false, false
	for !sawHeaders || !sawData {
		header := make([]byte, 9)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		length := int(header[0])<<16 | int(header[1])<<8 | int(header[2])
		frameType, flags := header[3], header[4]
		if _, err := io.CopyN(io.Discard, conn, int64(length)); err != nil {
			return
		}

		switch frameType {
		case frameSettings:
			if flags&flagAck == 0 {
				if _, err := conn.Write(frameHeader(0, frameSettings, flagAck, 0)); err != nil {
					return
				}
			}
		case frameHeaders:
			sawHeaders = true
		case frameData:
			sawData = true
		}
	}

	// GOAWAY with last-stream-id 0: the client's stream 1 was not processed.
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], 0)
	binary.BigEndian.PutUint32(payload[4:8], 0)
	_, _ = conn.Write(append(frameHeader(len(payload), frameGoAway, 0, 0), payload...))
	_ = conn.Close()
}

func frameHeader(length int, frameType, flags byte, streamID uint32) []byte {
	header := make([]byte, 9)
	header[0], header[1], header[2] = byte(length>>16), byte(length>>8), byte(length)
	header[3] = frameType
	header[4] = flags
	binary.BigEndian.PutUint32(header[5:9], streamID&0x7fffffff)

	return header
}

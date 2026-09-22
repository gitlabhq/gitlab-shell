package lfstransfer

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/git-lfs-transfer/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const responseCancellationTimeout = 5 * time.Second

// retryablehttp retries other 5xx responses, which would rerun this single-use handler and hang or panic.
var nonRetryableStatusCodes = []int{http.StatusBadRequest, http.StatusNotImplemented}

func newStreamingServer(t *testing.T, statusCode int) (*httptest.Server, <-chan struct{}) {
	t.Helper()

	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()

		<-r.Context().Done()
		close(requestCanceled)
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})

	return server, requestCanceled
}

func requireSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(responseCancellationTimeout):
		require.Fail(t, "timed out waiting for signal")
	}
}

func TestNewClient(t *testing.T) {
	client, err := NewClient(nil, nil, "https://example.com", "authorization")
	require.NoError(t, err)
	require.NotNil(t, client.client)
	require.Equal(t, 3, client.client.RetryMax)
	require.Nil(t, client.client.Logger)
}

func TestNewAuthenticatedPostRequest(t *testing.T) {
	client, err := NewClient(nil, nil, "", "custom-authorization")
	require.NoError(t, err)

	for _, tc := range []struct {
		desc    string
		url     string
		wantErr bool
	}{
		{
			desc: "authenticated request",
			url:  "https://example.com/locks",
		},
		{
			desc:    "invalid url",
			url:     "://example.com/locks",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			req, err := client.newAuthenticatedPostRequest(tc.url, strings.NewReader("body"))

			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, req)
				return
			}

			require.NoError(t, err)
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "https://example.com/locks", req.URL.String())
			require.Equal(t, ClientHeader, req.Header.Get("Content-Type"))
			require.Len(t, req.Header.Values("Content-Type"), 1)
			require.Equal(t, "custom-authorization", req.Header.Get("Authorization"))
			require.Len(t, req.Header.Values("Authorization"), 1)
		})
	}
}

func TestBatchClosesErrorResponseBody(t *testing.T) {
	requireClosesErrorResponseBody(t, func(t *testing.T, client *Client, statusCode int) {
		response, err := client.Batch("download", nil, "", "")

		require.ErrorContains(t, err, fmt.Sprintf("response failed with status code: %d", statusCode))
		require.Nil(t, response)
	})
}

func TestGetObjectClosesErrorResponseBody(t *testing.T) {
	requireClosesErrorResponseBody(t, func(t *testing.T, client *Client, _ int) {
		body, size, err := client.GetObject("", client.href, nil)

		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, body)
		require.Zero(t, size)
	})
}

func TestPutObjectStreamsBody(t *testing.T) {
	firstBytesRead := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstBytes := make([]byte, 3)
		_, err := io.ReadFull(r.Body, firstBytes)
		assert.NoError(t, err)
		close(firstBytesRead)

		_, err = io.Copy(io.Discard, r.Body)
		assert.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(nil, nil, server.URL, "authorization")
	require.NoError(t, err)

	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	requestResult := make(chan error, 1)
	go func() {
		requestResult <- client.PutObject("oid", server.URL, nil, 6, reader)
	}()

	_, err = writer.Write([]byte("abc"))
	require.NoError(t, err)
	requireSignal(t, firstBytesRead)
	_, err = writer.Write([]byte("def"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, <-requestResult)
}

func TestPutObjectSetsContentLength(t *testing.T) {
	const body = "object contents"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contents, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, int64(len(body)), r.ContentLength)
		assert.NotContains(t, r.TransferEncoding, "chunked")
		assert.Equal(t, body, string(contents))
		assert.Equal(t, "header value", r.Header.Get("Custom-Header"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(nil, nil, server.URL, "authorization")
	require.NoError(t, err)

	err = client.PutObject("oid", server.URL, map[string]string{"Custom-Header": "header value"}, int64(len(body)), strings.NewReader(body))
	require.NoError(t, err)
}

func TestPutObjectClosesErrorResponseBeforeWaitingForRequestBody(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serverDone := make(chan struct{})
	acceptedConnection := make(chan net.Conn, 1)
	t.Cleanup(func() {
		close(serverDone)
		_ = listener.Close()
		if connection := <-acceptedConnection; connection != nil {
			_ = connection.Close()
		}
	})

	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			acceptedConnection <- nil
			return
		}
		acceptedConnection <- connection

		reader := bufio.NewReader(connection)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}

		_, _ = io.WriteString(connection, "HTTP/1.1 403 Forbidden\r\nContent-Length: 6\r\n\r\ndenied")
		<-serverDone
	}()

	client, err := NewClient(nil, nil, "", "authorization")
	require.NoError(t, err)

	requestResult := make(chan error, 1)
	go func() {
		body := io.LimitReader(zeroReader{}, 1<<30)
		requestResult <- client.PutObject("oid", "http://"+listener.Addr().String(), nil, 1<<30, body)
	}()

	select {
	case err := <-requestResult:
		require.ErrorContains(t, err, "internal error (403)")
	case <-time.After(responseCancellationTimeout):
		require.FailNow(t, "PutObject did not return after receiving an error response")
	}
}

func TestPutObjectSendsZeroContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Zero(t, r.ContentLength)
		assert.NotContains(t, r.TransferEncoding, "chunked")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(nil, nil, server.URL, "authorization")
	require.NoError(t, err)

	err = client.PutObject("oid", server.URL, nil, 0, strings.NewReader(""))
	require.NoError(t, err)
}

func TestPutObjectRejectsNegativeSize(t *testing.T) {
	client, err := NewClient(nil, nil, "", "authorization")
	require.NoError(t, err)

	err = client.PutObject("oid", "https://example.com/object", nil, -1, strings.NewReader(""))
	require.EqualError(t, err, "invalid size: -1")
}

func TestPutObjectDoesNotRetry(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(nil, nil, server.URL, "authorization")
	require.NoError(t, err)

	err = client.PutObject("oid", server.URL, nil, 1, strings.NewReader("x"))
	require.ErrorContains(t, err, "internal error (500)")
	require.Equal(t, int32(1), requestCount.Load())
}

func TestPutObjectBodyErrorPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(nil, nil, server.URL, "authorization")
	require.NoError(t, err)

	body := io.MultiReader(strings.NewReader("partial"), failingReader{err: transfer.ErrCorruptData})
	err = client.PutObject("oid", server.URL, nil, 100, body)
	require.ErrorIs(t, err, transfer.ErrCorruptData)
}

func TestPutObjectWaitsForRequestBodyClose(t *testing.T) {
	client, err := NewClient(nil, nil, "", "authorization")
	require.NoError(t, err)

	releaseBody := make(chan struct{})
	bodyClosed := make(chan struct{})
	client.client.HTTPClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		go func() {
			<-releaseBody
			assert.NoError(t, req.Body.Close())
			close(bodyClosed)
		}()

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})

	requestResult := make(chan error, 1)
	go func() {
		requestResult <- client.PutObject("oid", "https://example.com/object", nil, 1, strings.NewReader("x"))
	}()

	select {
	case err := <-requestResult:
		require.FailNowf(t, "PutObject returned before the body was closed", "error: %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	close(releaseBody)
	requireSignal(t, bodyClosed)
	require.NoError(t, <-requestResult)
}

func TestPutObjectWaitsForBlockedRequestBodyRead(t *testing.T) {
	client, err := NewClient(nil, nil, "", "authorization")
	require.NoError(t, err)

	reader, writer := io.Pipe()
	readStarted := make(chan struct{})
	body := &notifyingReader{Reader: reader, started: readStarted}
	var goroutines sync.WaitGroup
	t.Cleanup(func() {
		_ = writer.Close()
		_ = reader.Close()
		goroutines.Wait()
	})

	closeStarted := make(chan struct{})
	readResult := make(chan error, 1)
	closeResult := make(chan error, 1)
	client.client.HTTPClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		goroutines.Add(2)
		go func() {
			defer goroutines.Done()
			_, readErr := req.Body.Read(make([]byte, 1))
			readResult <- readErr
		}()
		<-readStarted

		go func() {
			defer goroutines.Done()
			close(closeStarted)
			closeResult <- req.Body.Close()
		}()
		<-closeStarted

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})

	requestResult := make(chan error, 1)
	goroutines.Add(1)
	go func() {
		defer goroutines.Done()
		requestResult <- client.PutObject("oid", "https://example.com/object", nil, 1, body)
	}()

	select {
	case requestErr := <-requestResult:
		require.FailNowf(t, "PutObject returned while the body read was blocked", "error: %v", requestErr)
	case <-time.After(10 * time.Millisecond):
	}

	_, err = writer.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, <-readResult)
	require.NoError(t, <-closeResult)
	select {
	case requestErr := <-requestResult:
		require.NoError(t, requestErr)
	case <-time.After(responseCancellationTimeout):
		require.FailNow(t, "PutObject did not return after the body read completed")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type notifyingReader struct {
	io.Reader
	started chan struct{}
	once    sync.Once
}

func (r *notifyingReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.Reader.Read(p)
}

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

func requireClosesErrorResponseBody(t *testing.T, call func(*testing.T, *Client, int)) {
	t.Helper()

	for _, statusCode := range nonRetryableStatusCodes {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			server, requestCanceled := newStreamingServer(t, statusCode)
			client, err := NewClient(nil, nil, server.URL, "authorization")
			require.NoError(t, err)

			call(t, client, statusCode)
			requireSignal(t, requestCanceled)
		})
	}
}

package githttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
)

const (
	serviceName           = "git-test-pack"
	unexpectedResponseMsg = "unexpected git-test-pack response"
)

type capturedRequest struct {
	path    string
	headers http.Header
	body    []byte
}

func startCapturingServer(t *testing.T, responseBody string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.headers = r.Header.Clone()
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		captured.body = body
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(responseBody))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func TestWithGitProtocolHeader(t *testing.T) {
	testCases := []struct {
		desc    string
		headers map[string]string
		version string
		want    map[string]string
	}{
		{
			desc:    "nil headers",
			version: testGitProtocolVersion,
			want:    map[string]string{gitProtocolHeader: testGitProtocolVersion},
		},
		{
			desc:    "existing headers are preserved and not mutated",
			headers: map[string]string{"Existing-Header": "value"},
			version: testGitProtocolVersion,
			want:    map[string]string{"Existing-Header": "value", gitProtocolHeader: testGitProtocolVersion},
		},
		{
			desc:    "empty version is set",
			headers: map[string]string{gitProtocolHeader: testGitProtocolVersion},
			want:    map[string]string{gitProtocolHeader: ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			originalHeaders := maps.Clone(tc.headers)

			result := withGitProtocolHeader(tc.headers, tc.version)

			assert.Equal(t, tc.want, result)
			assert.Equal(t, originalHeaders, tc.headers)
		})
	}
}

type fakeInfoRefsClient struct {
	response   *http.Response
	err        error
	gotService string
	requestCtx context.Context
}

func (c *fakeInfoRefsClient) InfoRefs(ctx context.Context, service string) (*http.Response, error) {
	c.gotService = service
	c.requestCtx = ctx
	return c.response, c.err
}

func TestForInfoRefs(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		timeoutSeconds uint64
		wantTimeout    time.Duration
	}{
		{desc: "default timeout", wantTimeout: 300 * time.Second},
		{desc: "configured timeout", timeoutSeconds: 7, wantTimeout: 7 * time.Second},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := &config.Config{HTTPSettings: config.HTTPSettingsConfig{ReadTimeoutSeconds: tc.timeoutSeconds}}
			for _, command := range []struct {
				name    string
				command gitHTTPCommand
			}{
				{name: "pull", command: &PullCommand{Config: cfg}},
				{name: "push", command: &PushCommand{Config: cfg}},
			} {
				t.Run(command.name, func(t *testing.T) {
					_, _, _, timeout := command.command.ForInfoRefs()

					assert.Equal(t, tc.wantTimeout, timeout)
				})
			}
		})
	}
}

type fakeGitHTTPCommand struct {
	readWriter  *readwriter.ReadWriter
	serviceName string
	httpPrefix  []byte
	timeout     time.Duration
}

func (c fakeGitHTTPCommand) ForInfoRefs() (*readwriter.ReadWriter, string, []byte, time.Duration) {
	return c.readWriter, c.serviceName, c.httpPrefix, c.timeout
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestRequestInfoRefs(t *testing.T) {
	prefix := []byte("service-prefix")
	payload := []byte("advertised refs")
	fullBody := slices.Concat(prefix, payload)

	for _, tc := range []struct {
		desc       string
		body       io.Reader
		clientErr  error
		out        io.Writer
		wantOut    []byte
		wantErrMsg string
	}{
		{desc: "normal read", body: bytes.NewReader(fullBody), wantOut: payload},
		{desc: "one byte at a time", body: iotest.OneByteReader(bytes.NewReader(fullBody)), wantOut: payload},
		{desc: "rejects a mismatched prefix", body: bytes.NewReader([]byte("wrong-prefix!!")), wantErrMsg: unexpectedResponseMsg},
		{desc: "rejects a truncated prefix", body: bytes.NewReader(prefix[:len(prefix)-1]), wantErrMsg: unexpectedResponseMsg},
		{desc: "rejects an empty body", body: bytes.NewReader(nil), wantErrMsg: unexpectedResponseMsg},
		{desc: "returns the request error unchanged", clientErr: errors.New("info/refs request failed"), wantErrMsg: "info/refs request failed"},
		{desc: "returns an output error", body: bytes.NewReader(fullBody), out: failingWriter{err: errors.New("write failed")}, wantErrMsg: "write failed"},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			body := &trackingReadCloser{Reader: tc.body}
			output := &bytes.Buffer{}
			out := tc.out
			if out == nil {
				out = output
			}
			command := fakeGitHTTPCommand{
				readWriter:  &readwriter.ReadWriter{Out: out},
				serviceName: serviceName,
				httpPrefix:  prefix,
				timeout:     time.Second,
			}
			client := &fakeInfoRefsClient{response: &http.Response{Body: body}, err: tc.clientErr}

			err := requestInfoRefs(context.Background(), client, command)

			if tc.wantErrMsg == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.wantErrMsg)
			}
			assert.Equal(t, tc.wantOut, output.Bytes())
			assert.Equal(t, serviceName, client.gotService)
			if tc.clientErr == nil {
				assert.True(t, body.closed)
			}
		})
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) {
	return f(p)
}

func TestRequestInfoRefsTimeout(t *testing.T) {
	const timeout = 100 * time.Millisecond
	prefix := []byte("service-prefix")
	payload := []byte("advertised refs")

	t.Run("times out while reading a partial prefix", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*timeout)
			defer cancel()
			client := &fakeInfoRefsClient{}
			body := &trackingReadCloser{Reader: io.MultiReader(bytes.NewReader(prefix[:1]), readerFunc(func([]byte) (int, error) {
				<-client.requestCtx.Done()
				return 0, client.requestCtx.Err()
			}))}
			client.response = &http.Response{Body: body}
			command := fakeGitHTTPCommand{
				readWriter:  &readwriter.ReadWriter{Out: io.Discard},
				serviceName: serviceName,
				httpPrefix:  prefix,
				timeout:     timeout,
			}
			start := time.Now()

			err := requestInfoRefs(ctx, client, command)

			require.EqualError(t, err, unexpectedResponseMsg)
			assert.Equal(t, timeout, time.Since(start))
			assert.True(t, body.closed)
		})
	})

	t.Run("rejects a complete prefix returned after the timer expires", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			prefixReader := bytes.NewReader(prefix)
			body := &trackingReadCloser{Reader: readerFunc(func(p []byte) (int, error) {
				time.Sleep(2 * timeout)
				return prefixReader.Read(p)
			})}
			client := &fakeInfoRefsClient{response: &http.Response{Body: body}}
			command := fakeGitHTTPCommand{
				readWriter:  &readwriter.ReadWriter{Out: io.Discard},
				serviceName: serviceName,
				httpPrefix:  prefix,
				timeout:     timeout,
			}

			err := requestInfoRefs(context.Background(), client, command)

			require.EqualError(t, err, unexpectedResponseMsg)
			assert.True(t, body.closed)
		})
	})

	t.Run("cancels a stalled git.Client response body", func(t *testing.T) {
		releaseHandler := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write(prefix[:1])
			assert.NoError(t, err)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-releaseHandler:
			}
		}))
		// Cleanups run in reverse order, so the handler is released before
		// server.Close waits for it to return.
		t.Cleanup(server.Close)
		t.Cleanup(func() { close(releaseHandler) })
		command := fakeGitHTTPCommand{
			readWriter:  &readwriter.ReadWriter{Out: io.Discard},
			serviceName: serviceName,
			httpPrefix:  prefix,
			timeout:     timeout,
		}
		result := make(chan error, 1)

		go func() {
			result <- requestInfoRefs(context.Background(), &git.Client{URL: server.URL}, command)
		}()

		select {
		case err := <-result:
			require.EqualError(t, err, unexpectedResponseMsg)
		case <-time.After(5 * time.Second):
			t.Fatal("requestInfoRefs did not return after the info/refs timeout")
		}
	})

	t.Run("does not time out while copying the payload", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			client := &fakeInfoRefsClient{}
			payloadReader := bytes.NewReader(payload)
			body := &trackingReadCloser{Reader: io.MultiReader(bytes.NewReader(prefix), readerFunc(func(p []byte) (int, error) {
				time.Sleep(2 * timeout)
				if err := client.requestCtx.Err(); err != nil {
					return 0, err
				}
				return payloadReader.Read(p)
			}))}
			client.response = &http.Response{Body: body}
			output := &bytes.Buffer{}
			command := fakeGitHTTPCommand{
				readWriter:  &readwriter.ReadWriter{Out: output},
				serviceName: serviceName,
				httpPrefix:  prefix,
				timeout:     timeout,
			}

			err := requestInfoRefs(context.Background(), client, command)

			require.NoError(t, err)
			assert.Equal(t, payload, output.Bytes())
			assert.True(t, body.closed)
		})
	})
}

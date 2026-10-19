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
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpclient "gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
)

const (
	serviceName           = "git-test-pack"
	unexpectedResponseMsg = "unexpected git-test-pack response"
	repoUnavailableMsg    = "Remote repository is unavailable"
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
	response            *http.Response
	err                 error
	waitForCancellation bool
	gotService          string
	requestCtx          context.Context
}

func (c *fakeInfoRefsClient) InfoRefs(ctx context.Context, service string) (*http.Response, error) {
	c.gotService = service
	c.requestCtx = ctx
	if c.waitForCancellation {
		<-ctx.Done()
	}
	return c.response, c.err
}

type fakeGitHTTPCommand struct {
	readWriter  *readwriter.ReadWriter
	serviceName string
	httpPrefix  []byte
}

func (c fakeGitHTTPCommand) ForInfoRefs() (*readwriter.ReadWriter, string, []byte) {
	return c.readWriter, c.serviceName, c.httpPrefix
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
		desc      string
		body      io.Reader
		clientErr error
		out       io.Writer
		wantOut   []byte
		wantErr   string
	}{
		{desc: "normal read", body: bytes.NewReader(fullBody), wantOut: payload},
		{desc: "rejects a mismatched prefix", body: bytes.NewReader([]byte("wrong-prefix!!")), wantErr: unexpectedResponseMsg},
		{desc: "rejects a truncated prefix", body: bytes.NewReader(prefix[:len(prefix)-1]), wantErr: unexpectedResponseMsg},
		{desc: "rejects an empty body", body: bytes.NewReader(nil), wantErr: unexpectedResponseMsg},
		{desc: "returns the request error unchanged", clientErr: errors.New("info/refs request failed"), wantErr: "info/refs request failed"},
		{desc: "returns the request API error unchanged", clientErr: &httpclient.APIError{Msg: repoUnavailableMsg}, wantErr: repoUnavailableMsg},
		{desc: "returns an output error", body: bytes.NewReader(fullBody), out: failingWriter{err: errors.New("write failed")}, wantErr: "write failed"},
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
			}
			client := &fakeInfoRefsClient{response: &http.Response{Body: body}, err: tc.clientErr}

			err := requestInfoRefs(context.Background(), client, command, time.Second)

			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.wantErr)
			}
			assert.Equal(t, tc.wantOut, output.Bytes())
			assert.Equal(t, serviceName, client.gotService)
			if tc.clientErr == nil {
				assert.True(t, body.closed)
			} else {
				assert.Same(t, tc.clientErr, err)
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
	const timedOutMsg = "git-test-pack info/refs timed out after 100ms"
	prefix := []byte("service-prefix")
	payload := []byte("advertised refs")

	t.Run("times out while waiting for the response headers", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*timeout)
			defer cancel()
			client := &fakeInfoRefsClient{
				err:                 &httpclient.APIError{Msg: repoUnavailableMsg},
				waitForCancellation: true,
			}
			output := &bytes.Buffer{}
			command := fakeGitHTTPCommand{readWriter: &readwriter.ReadWriter{Out: output}, serviceName: serviceName, httpPrefix: prefix}
			start := time.Now()

			err := requestInfoRefs(ctx, client, command, timeout)

			require.EqualError(t, err, timedOutMsg)
			assert.Equal(t, timeout, time.Since(start))
			assert.Empty(t, output.Bytes())
		})
	})

	t.Run("returns the request error unchanged when the parent context is canceled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &fakeInfoRefsClient{
				err:                 &httpclient.APIError{Msg: repoUnavailableMsg},
				waitForCancellation: true,
			}
			output := &bytes.Buffer{}
			command := fakeGitHTTPCommand{readWriter: &readwriter.ReadWriter{Out: output}, serviceName: serviceName, httpPrefix: prefix}
			start := time.Now()
			go func() {
				time.Sleep(timeout / 2)
				cancel()
			}()

			err := requestInfoRefs(ctx, client, command, timeout)

			require.Same(t, client.err, err)
			assert.Equal(t, timeout/2, time.Since(start))
			assert.Empty(t, output.Bytes())
		})
	})

	t.Run("times out while waiting for the prefix", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*timeout)
			defer cancel()
			client := &fakeInfoRefsClient{}
			body := &trackingReadCloser{Reader: readerFunc(func([]byte) (int, error) {
				<-client.requestCtx.Done()
				return 0, client.requestCtx.Err()
			})}
			client.response = &http.Response{Body: body}
			output := &bytes.Buffer{}
			command := fakeGitHTTPCommand{readWriter: &readwriter.ReadWriter{Out: output}, serviceName: serviceName, httpPrefix: prefix}
			start := time.Now()

			err := requestInfoRefs(ctx, client, command, timeout)

			require.EqualError(t, err, timedOutMsg)
			assert.Equal(t, timeout, time.Since(start))
			assert.Empty(t, output.Bytes())
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
			output := &bytes.Buffer{}
			command := fakeGitHTTPCommand{readWriter: &readwriter.ReadWriter{Out: output}, serviceName: serviceName, httpPrefix: prefix}

			err := requestInfoRefs(context.Background(), client, command, timeout)

			require.EqualError(t, err, timedOutMsg)
			assert.Empty(t, output.Bytes())
			assert.True(t, body.closed)
		})
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
			command := fakeGitHTTPCommand{readWriter: &readwriter.ReadWriter{Out: output}, serviceName: serviceName, httpPrefix: prefix}

			err := requestInfoRefs(context.Background(), client, command, timeout)

			require.NoError(t, err)
			assert.Equal(t, payload, output.Bytes())
			assert.True(t, body.closed)
		})
	})
}

type executableCommand interface {
	Execute(context.Context) error
}

func TestExecuteBoundsInfoRefsByReadTimeout(t *testing.T) {
	for _, command := range []struct {
		service    string
		httpPrefix []byte
		build      func(*config.Config, *readwriter.ReadWriter, *accessverifier.Response) executableCommand
	}{
		{
			service:    pullService,
			httpPrefix: uploadPackHTTPPrefix,
			build: func(cfg *config.Config, rw *readwriter.ReadWriter, response *accessverifier.Response) executableCommand {
				return &PullCommand{Config: cfg, ReadWriter: rw, Response: response}
			},
		},
		{
			service:    pushService,
			httpPrefix: receivePackHTTPPrefix,
			build: func(cfg *config.Config, rw *readwriter.ReadWriter, response *accessverifier.Response) executableCommand {
				return &PushCommand{Config: cfg, ReadWriter: rw, Response: response}
			},
		},
	} {
		t.Run(command.service, func(t *testing.T) {
			t.Parallel()

			for _, tc := range []struct {
				desc               string
				readTimeoutSeconds uint64
				prefixStall        time.Duration
				wantErr            string
				wantOut            string
			}{
				{
					desc:               "configured read_timeout cancels a stalled prefix",
					readTimeoutSeconds: 1,
					prefixStall:        time.Minute,
					wantErr:            command.service + " info/refs timed out after 1s",
				},
				{
					desc:        "zero read_timeout falls back to the default",
					prefixStall: 100 * time.Millisecond,
					wantOut:     infoRefsWithoutPrefix,
				},
			} {
				t.Run(tc.desc, func(t *testing.T) {
					t.Parallel()

					url := testserver.StartHTTPServer(t, []testserver.TestRequestHandler{
						{
							Path: testInfoRefsPath,
							Handler: func(w http.ResponseWriter, r *http.Request) {
								w.WriteHeader(http.StatusOK)
								w.(http.Flusher).Flush()

								select {
								case <-r.Context().Done():
									return
								case <-time.After(tc.prefixStall):
								}

								_, err := w.Write(slices.Concat(command.httpPrefix, []byte(infoRefsWithoutPrefix)))
								assert.NoError(t, err)
							},
						},
						{
							Path: "/" + command.service,
							Handler: func(_ http.ResponseWriter, r *http.Request) {
								_, err := io.Copy(io.Discard, r.Body)
								assert.NoError(t, err)
							},
						},
					})
					output := &bytes.Buffer{}
					cmd := command.build(
						&config.Config{HTTPSettings: config.HTTPSettingsConfig{ReadTimeoutSeconds: tc.readTimeoutSeconds}},
						&readwriter.ReadWriter{Out: output, In: strings.NewReader("")},
						&accessverifier.Response{Payload: accessverifier.CustomPayload{Data: accessverifier.CustomPayloadData{PrimaryRepo: url}}},
					)

					start := time.Now()

					err := cmd.Execute(context.Background())

					if tc.wantErr == "" {
						require.NoError(t, err)
					} else {
						require.EqualError(t, err, tc.wantErr)
						elapsed := time.Since(start)
						assert.GreaterOrEqual(t, elapsed, time.Duration(tc.readTimeoutSeconds)*time.Second)
						assert.Less(t, elapsed, 10*time.Second)
					}
					assert.Equal(t, tc.wantOut, output.String())
				})
			}
		})
	}
}

package githttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/git"
)

type fakeInfoRefsClient struct {
	response *http.Response
	err      error
}

func (c fakeInfoRefsClient) InfoRefs(context.Context, string) (*http.Response, error) {
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

func TestSetGitProtocolHeader(t *testing.T) {
	t.Run("allocates omitted headers", func(t *testing.T) {
		client := &git.Client{}

		setGitProtocolHeader(client, "version=2")

		assert.Equal(t, map[string]string{gitProtocolHeader: "version=2"}, client.Headers)
	})

	t.Run("preserves existing headers", func(t *testing.T) {
		client := &git.Client{Headers: map[string]string{"Existing-Header": "value"}}

		setGitProtocolHeader(client, "version=2")

		assert.Equal(t, map[string]string{"Existing-Header": "value", gitProtocolHeader: "version=2"}, client.Headers)
	})

	t.Run("stores an empty protocol version", func(t *testing.T) {
		client := &git.Client{}

		setGitProtocolHeader(client, "")

		assert.Equal(t, map[string]string{gitProtocolHeader: ""}, client.Headers)
	})
}

func TestRequestInfoRefs(t *testing.T) {
	const serviceName = "git-test-pack"
	prefix := []byte("service-prefix")
	payload := []byte("advertised refs")
	fullBody := slices.Concat(prefix, payload)

	t.Run("reads a complete prefix delivered one byte at a time", func(t *testing.T) {
		body := &trackingReadCloser{Reader: iotest.OneByteReader(bytes.NewReader(fullBody))}
		output := &bytes.Buffer{}
		command := fakeGitHTTPCommand{
			readWriter:  &readwriter.ReadWriter{Out: output},
			serviceName: serviceName,
			httpPrefix:  prefix,
		}

		err := requestInfoRefs(context.Background(), fakeInfoRefsClient{response: &http.Response{Body: body}}, command)

		require.NoError(t, err)
		assert.Equal(t, payload, output.Bytes())
		assert.True(t, body.closed)
	})

	t.Run("strips the prefix from a normal response", func(t *testing.T) {
		body := &trackingReadCloser{Reader: bytes.NewReader(fullBody)}
		output := &bytes.Buffer{}
		command := fakeGitHTTPCommand{
			readWriter:  &readwriter.ReadWriter{Out: output},
			serviceName: serviceName,
			httpPrefix:  prefix,
		}

		err := requestInfoRefs(context.Background(), fakeInfoRefsClient{response: &http.Response{Body: body}}, command)

		require.NoError(t, err)
		assert.Equal(t, payload, output.Bytes())
		assert.True(t, body.closed)
	})

	for _, tc := range []struct {
		desc string
		body []byte
	}{
		{desc: "rejects a mismatched prefix", body: []byte("wrong-prefix!!")},
		{desc: "rejects a truncated prefix", body: prefix[:len(prefix)-1]},
		{desc: "rejects an empty body", body: nil},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			body := &trackingReadCloser{Reader: bytes.NewReader(tc.body)}
			command := fakeGitHTTPCommand{
				readWriter:  &readwriter.ReadWriter{Out: io.Discard},
				serviceName: serviceName,
				httpPrefix:  prefix,
			}

			err := requestInfoRefs(context.Background(), fakeInfoRefsClient{response: &http.Response{Body: body}}, command)

			require.EqualError(t, err, "unexpected git-test-pack response")
			assert.True(t, body.closed)
		})
	}

	t.Run("returns the request error unchanged", func(t *testing.T) {
		requestError := errors.New("request failed")
		command := fakeGitHTTPCommand{readWriter: &readwriter.ReadWriter{}, serviceName: serviceName, httpPrefix: prefix}

		err := requestInfoRefs(context.Background(), fakeInfoRefsClient{err: requestError}, command)

		require.ErrorIs(t, err, requestError)
	})

	t.Run("returns an output error", func(t *testing.T) {
		writeError := errors.New("write failed")
		body := &trackingReadCloser{Reader: bytes.NewReader(fullBody)}
		command := fakeGitHTTPCommand{
			readWriter:  &readwriter.ReadWriter{Out: failingWriter{err: writeError}},
			serviceName: serviceName,
			httpPrefix:  prefix,
		}

		err := requestInfoRefs(context.Background(), fakeInfoRefsClient{response: &http.Response{Body: body}}, command)

		require.ErrorIs(t, err, writeError)
		assert.True(t, body.closed)
	})
}

func TestExecuteSSHRequest(t *testing.T) {
	t.Run("forwards the request and response", func(t *testing.T) {
		body := &trackingReadCloser{Reader: strings.NewReader("response")}
		output := &bytes.Buffer{}
		var requestBody []byte
		requestFn := func(_ context.Context, input io.Reader) (*http.Response, error) {
			var err error
			requestBody, err = io.ReadAll(input)
			require.NoError(t, err)
			return &http.Response{Body: body}, nil
		}

		err := executeSSHRequest(context.Background(), requestFn, &readwriter.ReadWriter{In: strings.NewReader("request"), Out: output})

		require.NoError(t, err)
		assert.Equal(t, []byte("request"), requestBody)
		assert.Equal(t, "response", output.String())
		assert.True(t, body.closed)
	})

	t.Run("returns a request error without copying", func(t *testing.T) {
		requestError := errors.New("request failed")
		output := &bytes.Buffer{}
		requestFn := func(context.Context, io.Reader) (*http.Response, error) {
			return nil, requestError
		}

		err := executeSSHRequest(context.Background(), requestFn, &readwriter.ReadWriter{In: strings.NewReader("request"), Out: output})

		require.ErrorIs(t, err, requestError)
		assert.Empty(t, output.Bytes())
	})

	t.Run("returns a response copy error", func(t *testing.T) {
		writeError := errors.New("write failed")
		body := &trackingReadCloser{Reader: strings.NewReader("response")}
		requestFn := func(context.Context, io.Reader) (*http.Response, error) {
			return &http.Response{Body: body}, nil
		}

		err := executeSSHRequest(context.Background(), requestFn, &readwriter.ReadWriter{In: strings.NewReader("request"), Out: failingWriter{err: writeError}})

		require.ErrorIs(t, err, writeError)
		assert.True(t, body.closed)
	})
}

func TestPipeRequest(t *testing.T) {
	t.Run("forwards the request and response", func(t *testing.T) {
		body := &trackingReadCloser{Reader: strings.NewReader("response")}
		output := &bytes.Buffer{}
		var requestBody []byte
		writeFn := func(input io.Reader, writer *io.PipeWriter) {
			defer func() { _ = writer.Close() }()
			_, _ = io.Copy(writer, input)
		}
		requestFn := func(_ context.Context, input io.Reader) (*http.Response, error) {
			var err error
			requestBody, err = io.ReadAll(input)
			require.NoError(t, err)
			return &http.Response{Body: body}, nil
		}

		err := pipeRequest(context.Background(), &readwriter.ReadWriter{In: strings.NewReader("request"), Out: output}, writeFn, requestFn)

		require.NoError(t, err)
		assert.Equal(t, []byte("request"), requestBody)
		assert.Equal(t, "response", output.String())
		assert.True(t, body.closed)
	})

	t.Run("returns a request error", func(t *testing.T) {
		requestError := errors.New("request failed")
		writeFn := func(_ io.Reader, writer *io.PipeWriter) {
			_ = writer.Close()
		}
		requestFn := func(context.Context, io.Reader) (*http.Response, error) {
			return nil, requestError
		}

		err := pipeRequest(context.Background(), &readwriter.ReadWriter{In: strings.NewReader("request"), Out: io.Discard}, writeFn, requestFn)

		require.ErrorIs(t, err, requestError)
	})

	t.Run("returns a response copy error", func(t *testing.T) {
		writeError := errors.New("write failed")
		body := &trackingReadCloser{Reader: strings.NewReader("response")}
		writeFn := func(_ io.Reader, writer *io.PipeWriter) {
			_ = writer.Close()
		}
		requestFn := func(_ context.Context, input io.Reader) (*http.Response, error) {
			_, err := io.ReadAll(input)
			require.NoError(t, err)
			return &http.Response{Body: body}, nil
		}

		err := pipeRequest(context.Background(), &readwriter.ReadWriter{In: strings.NewReader("request"), Out: failingWriter{err: writeError}}, writeFn, requestFn)

		require.ErrorIs(t, err, writeError)
		assert.True(t, body.closed)
	})
}

func TestReadUploadPackRequest(t *testing.T) {
	testCases := []struct {
		desc     string
		input    string
		expected string
	}{
		{
			desc:     "adds one flush after done and ignores trailing bytes",
			input:    pktLine("want ref\n") + pktLine("done\n") + "ignored",
			expected: pktLine("want ref\n") + pktLine("done\n") + flush,
		},
		{
			desc:     "forwards protocol v2 ls-refs through EOF",
			input:    lsRefsV2Request,
			expected: lsRefsV2Request,
		},
		{
			desc:     "fetch v2: stops at done and synthesizes trailing flush",
			input:    fetchV2Request,
			expected: fetchV2Request,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			output := capturePipeOutput(t, strings.NewReader(tc.input), readUploadPackRequest)
			assert.Equal(t, tc.expected, output)
		})
	}
}

func TestReadReceivePackRequest(t *testing.T) {
	oldOID := strings.Repeat("1", 40)
	newOID := strings.Repeat("2", 40)
	update := pktLine(oldOID + " " + newOID + " refs/heads/main\x00report-status\n")
	deletion := pktLine(oldOID + " " + strings.Repeat("0", 40) + " refs/heads/old\x00report-status\n")

	testCases := []struct {
		desc   string
		inputs []string
	}{
		{
			desc:   "forwards an update and pack data",
			inputs: []string{update, flush, "PACK data"},
		},
		{
			desc:   "forwards a deletion without pack data",
			inputs: []string{deletion + flush},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			readers := make([]io.Reader, 0, len(tc.inputs))
			for _, s := range tc.inputs {
				readers = append(readers, strings.NewReader(s))
			}

			output := capturePipeOutput(t, io.MultiReader(readers...), readReceivePackRequest)
			assert.Equal(t, strings.Join(tc.inputs, ""), output)
		})
	}
}

func capturePipeOutput(t *testing.T, input io.Reader, readFn func(io.Reader, *io.PipeWriter)) string {
	t.Helper()

	reader, writer := io.Pipe()
	go readFn(input, writer)

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	return string(output)
}

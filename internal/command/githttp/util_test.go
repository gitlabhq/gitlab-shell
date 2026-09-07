package githttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
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

type capturedSSHRequest struct {
	body          string
	gitProtocol   string
	authorization string
}

func setupSSHServer(t *testing.T, path, response string) (string, *capturedSSHRequest) {
	t.Helper()

	captured := &capturedSSHRequest{}
	url := testserver.StartHTTPServer(t, []testserver.TestRequestHandler{
		{
			Path: path,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				captured.body = string(body)
				captured.gitProtocol = r.Header.Get(gitProtocolHeader)
				captured.authorization = r.Header.Get(testAuthorizationHeader)
				_, err = w.Write([]byte(response))
				assert.NoError(t, err)
			},
		},
	})

	return url, captured
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
}

func TestRequestInfoRefs(t *testing.T) {
	const serviceName = "git-test-pack"
	prefix := []byte("service-prefix")
	payload := []byte("advertised refs")
	fullBody := slices.Concat(prefix, payload)

	for _, tc := range []struct {
		desc string
		wrap func(io.Reader) io.Reader
	}{
		{desc: "normal read", wrap: func(r io.Reader) io.Reader { return r }},
		{desc: "one byte at a time", wrap: iotest.OneByteReader},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			body := &trackingReadCloser{Reader: tc.wrap(bytes.NewReader(fullBody))}
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
	}

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

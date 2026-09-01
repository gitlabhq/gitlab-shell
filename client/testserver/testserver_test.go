package testserver

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestDecodeJSON(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		body := &trackingReadCloser{Reader: bytes.NewBufferString(`{"name":"Ada"}`)}
		request := &http.Request{Body: body}
		destination := &struct {
			Name string `json:"name"`
		}{}

		err := DecodeJSON(request, destination)

		require.NoError(t, err)
		assert.Equal(t, "Ada", destination.Name)
		assert.True(t, body.closed)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		body := &trackingReadCloser{Reader: bytes.NewBufferString(`{"name":`)}
		request := &http.Request{Body: body}

		err := DecodeJSON(request, &struct{}{})

		require.Error(t, err)
		assert.True(t, body.closed)
	})

	t.Run("reader error", func(t *testing.T) {
		readErr := errors.New("read failed")
		body := &trackingReadCloser{Reader: errorReader{err: readErr}}
		request := &http.Request{Body: body}

		err := DecodeJSON(request, &struct{}{})

		require.ErrorIs(t, err, readErr)
		assert.True(t, body.closed)
	})
}

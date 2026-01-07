// Package readwriter provides I/O abstractions for command input and output streams.
package readwriter

import (
	"io"
)

// ReadWriter bundles the standard input, output, and error streams for command execution.
type ReadWriter struct {
	Out    io.Writer
	In     io.Reader
	ErrOut io.Writer
}

// CountingWriter wraps an io.Writer and counts all the writes. Accessing
// the count N is not thread-safe.
type CountingWriter struct {
	W io.Writer
	N int64
}

func (cw *CountingWriter) Write(p []byte) (int, error) {
	n, err := cw.W.Write(p)
	cw.N += int64(n)
	return n, err
}

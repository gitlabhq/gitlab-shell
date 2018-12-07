package helper

import (
	"io"
)

// NewReceiveReader turns receiver into an io.Reader. Errors from the
// receiver function are passed on unmodified. This means receiver should
// emit io.EOF when done.
func NewReceiveReader(receiver func() ([]byte, error)) io.Reader {
	return &receiveReader{receiver: receiver}
}

type receiveReader struct {
	receiver func() ([]byte, error)
	data     []byte
	err      error
}

func (rr *receiveReader) Read(p []byte) (int, error) {
	if len(rr.data) == 0 {
		rr.data, rr.err = rr.receiver()
	}
	n := copy(p, rr.data)
	rr.data = rr.data[n:]
	if len(rr.data) == 0 {
		return n, rr.err
	}
	return n, nil
}

// NewSendWriter turns sender into an io.Writer. The number of 'bytes
// written' reported back is always len(p).
func NewSendWriter(sender func(p []byte) error) io.Writer {
	return &sendWriter{sender: sender}
}

type sendWriter struct {
	sender func([]byte) error
}

func (sw *sendWriter) Write(p []byte) (int, error) {
	return len(p), sw.sender(p)
}

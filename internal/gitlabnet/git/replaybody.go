package git

import (
	"errors"
	"io"
	"sync"
)

const (
	// DefaultReplayLimit is the most request-body bytes kept for replay. Git
	// negotiation for a pull and the commands plus pack of a small push fit
	// easily; larger bodies stream without replay, as before.
	DefaultReplayLimit = 1 << 20

	// replayWindow bounds how far the writer may run ahead of the current
	// reader, so a stalled HTTP request cannot buffer unbounded SSH input.
	replayWindow = 256 << 10
)

var errReplayExceeded = errors.New("request body exceeded the replay limit")

// bodyRewinder is a request body that can be re-opened from the start, which
// lets the HTTP transport retry the request on another connection.
type bodyRewinder interface {
	GetBody() (io.ReadCloser, error)
}

// ReplayBody is a pipe whose read side can be re-opened from the start, so a
// streaming request body stays retryable.
//
// Go's HTTP transport retries a request on a new connection when the pooled
// one dies before the request was processed (an idle close, an HTTP/2 GOAWAY,
// a REFUSED_STREAM), but only if it can replay the body: it needs Request.
// GetBody. An io.Pipe fed from SSH stdin cannot be replayed, so those retries
// failed with "net/http: cannot rewind body after connection loss". ReplayBody
// records what was consumed, up to a limit, and hands out fresh readers.
//
// The write side is used by the goroutine forwarding SSH stdin. Construction
// and each call to GetBody return an independent reader that starts at the
// first byte; the transport closes the old one before replaying. Readers block
// for data, and the writer blocks when it is replayWindow ahead of the newest
// reader. Abort unblocks everyone once the request is over.
type ReplayBody struct {
	mu   sync.Mutex
	cond *sync.Cond

	buf     []byte // bytes [base, written)
	base    int64  // stream offset of buf[0]
	written int64
	limit   int64

	writeClosed bool
	writeErr    error // error to surface to readers at EOF, if any
	aborted     bool

	current *replayReader // newest reader; drives backpressure and trimming
}

// NewReplayBody returns a ReplayBody that keeps up to limit bytes for replay
// and its first reader.
func NewReplayBody(limit int) (*ReplayBody, io.ReadCloser) {
	b := &ReplayBody{limit: int64(limit)}
	b.cond = sync.NewCond(&b.mu)
	b.current = &replayReader{body: b}

	return b, b.current
}

// Write appends p for readers. It blocks while the newest reader is
// replayWindow or more behind. It fails with io.ErrClosedPipe after Close or Abort.
func (b *ReplayBody) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for {
		if b.aborted || b.writeClosed {
			return 0, io.ErrClosedPipe
		}
		if b.written-b.current.pos < replayWindow {
			break
		}
		b.cond.Wait()
	}

	b.buf = append(b.buf, p...)
	b.written += int64(len(p))
	b.trimLocked()
	b.cond.Broadcast()

	return len(p), nil
}

// Close ends the stream: readers see io.EOF once they consume everything.
func (b *ReplayBody) Close() error {
	return b.CloseWithError(nil)
}

// CloseWithError ends the stream and makes readers return err instead of
// io.EOF once they consume everything.
func (b *ReplayBody) CloseWithError(err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.writeClosed {
		b.writeClosed = true
		b.writeErr = err
	}
	b.cond.Broadcast()

	return nil
}

// Abort tears the pipe down: pending and future Writes and Reads fail with
// io.ErrClosedPipe. Call it when the request is finished so the writer never
// blocks on a reader that will not come.
func (b *ReplayBody) Abort() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.aborted = true
	b.cond.Broadcast()
}

// GetBody is the http.Request.GetBody hook. It fails once more than the
// replay limit has been written, which makes the transport give up the retry
// exactly as it did before, instead of replaying a truncated body. The check
// and the new reader happen under one lock, so a concurrent Write cannot trim
// data the returned reader needs.
func (b *ReplayBody) GetBody() (io.ReadCloser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.written > b.limit {
		return nil, errReplayExceeded
	}

	r := &replayReader{body: b}
	b.current = r
	b.cond.Broadcast()

	return r, nil
}

// trimLocked drops bytes no reader can need: everything before the newest
// reader, once replay is off the table. It reslices rather than copies; append
// reallocates once capacity runs out and the dropped prefix is then freed, so
// memory stays bounded by the window.
func (b *ReplayBody) trimLocked() {
	// written only grows, so exceeding the limit permanently rules out replay.
	if b.written <= b.limit {
		return
	}
	pos := b.current.pos
	if drop := pos - b.base; drop > 0 {
		b.buf = b.buf[drop:]
		b.base = pos
	}
}

type replayReader struct {
	body   *ReplayBody
	pos    int64
	closed bool
}

func (r *replayReader) Read(p []byte) (int, error) {
	b := r.body
	b.mu.Lock()
	defer b.mu.Unlock()

	for {
		if r.closed || b.aborted {
			return 0, io.ErrClosedPipe
		}
		if r.pos < b.base {
			// Data this reader needs was trimmed: it is stale and cannot be
			// replayed.
			return 0, errReplayExceeded
		}
		if r.pos < b.written {
			n := copy(p, b.buf[r.pos-b.base:])
			r.pos += int64(n)
			if b.current == r {
				b.trimLocked()
			}
			b.cond.Broadcast()

			return n, nil
		}
		if b.writeClosed {
			if b.writeErr != nil {
				return 0, b.writeErr
			}

			return 0, io.EOF
		}
		b.cond.Wait()
	}
}

// GetBody re-opens the stream from the start; see ReplayBody.GetBody.
func (r *replayReader) GetBody() (io.ReadCloser, error) {
	return r.body.GetBody()
}

// Close marks this reader done. It does not end the stream, so a replay
// reader can still start from the beginning. It wakes a Read blocked in
// this reader, which the transport relies on when it abandons a request body.
func (r *replayReader) Close() error {
	b := r.body
	b.mu.Lock()
	defer b.mu.Unlock()

	r.closed = true
	b.cond.Broadcast()

	return nil
}

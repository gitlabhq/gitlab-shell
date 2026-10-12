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

// RetryableBody is a request body that can be re-opened from the start, which
// lets the HTTP transport retry the request on another connection. Readers
// returned by ReplayBody.Reader implement it.
type RetryableBody interface {
	io.ReadCloser
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
// The write side is used by the goroutine forwarding SSH stdin. Each call to
// Reader or GetBody returns an independent reader that starts at the first
// byte; the transport closes the old one before replaying. Readers block for
// data, and the writer blocks when it is replayWindow ahead of the newest
// reader. Abort unblocks everyone once the request is over.
type ReplayBody struct {
	mu   sync.Mutex
	cond *sync.Cond

	buf     []byte // bytes [base, written)
	base    int64  // stream offset of buf[0]
	written int64
	limit   int64

	replayable  bool
	writeClosed bool
	writeErr    error // error to surface to readers at EOF, if any
	aborted     bool

	current *replayReader // newest reader; drives backpressure and trimming
}

// NewReplayBody returns a ReplayBody that keeps up to limit bytes for replay.
func NewReplayBody(limit int) *ReplayBody {
	b := &ReplayBody{limit: int64(limit), replayable: true}
	b.cond = sync.NewCond(&b.mu)

	return b
}

// Write appends p for readers. It blocks while the newest reader is more than
// replayWindow behind. It fails with io.ErrClosedPipe after Close or Abort.
func (b *ReplayBody) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for {
		if b.aborted || b.writeClosed {
			return 0, io.ErrClosedPipe
		}
		if b.current == nil || b.written-b.current.pos < replayWindow {
			break
		}
		b.cond.Wait()
	}

	b.buf = append(b.buf, p...)
	b.written += int64(len(p))
	if b.replayable && b.written > b.limit {
		b.replayable = false
	}
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

// Reader returns a new reader positioned at the start of the stream. It
// becomes the reader that controls backpressure.
func (b *ReplayBody) Reader() RetryableBody {
	b.mu.Lock()
	defer b.mu.Unlock()

	r := &replayReader{body: b}
	b.current = r
	b.cond.Broadcast()

	return r
}

// GetBody is the http.Request.GetBody hook. It fails once more than the
// replay limit has been written, which makes the transport give up the retry
// exactly as it did before, instead of replaying a truncated body.
func (b *ReplayBody) GetBody() (io.ReadCloser, error) {
	b.mu.Lock()
	replayable := b.replayable
	b.mu.Unlock()

	if !replayable {
		return nil, errReplayExceeded
	}

	return b.Reader(), nil
}

// Replayable reports whether GetBody can still re-open the stream.
func (b *ReplayBody) Replayable() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.replayable
}

// trimLocked drops bytes no reader can need: everything before the newest
// reader, once replay is off the table. Callers hold b.mu.
func (b *ReplayBody) trimLocked() {
	if b.replayable || b.current == nil {
		return
	}
	drop := b.current.pos - b.base
	if drop <= 0 {
		return
	}
	b.buf = append(b.buf[:0], b.buf[drop:]...)
	b.base = b.current.pos
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
			// replayed. Only a reader created after the limit was exceeded, or
			// an old reader still being read, can get here.
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

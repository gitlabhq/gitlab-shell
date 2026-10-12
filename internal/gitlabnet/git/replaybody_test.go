package git

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReplayBodyStreamsAndReplays(t *testing.T) {
	body := NewReplayBody(1024)
	first := body.Reader()

	_, err := body.Write([]byte("0032want deadbeef\n"))
	require.NoError(t, err)

	buf := make([]byte, 64)
	n, err := first.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "0032want deadbeef\n", string(buf[:n]))

	_, err = body.Write([]byte("0000"))
	require.NoError(t, err)
	require.NoError(t, body.Close())

	rest, err := io.ReadAll(first)
	require.NoError(t, err)
	require.Equal(t, "0000", string(rest))

	// A replay reader starts from the first byte, after the original was
	// consumed to EOF and closed.
	require.NoError(t, first.Close())
	replay, err := first.GetBody()
	require.NoError(t, err)
	all, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, "0032want deadbeef\n0000", string(all))
}

func TestReplayBodyGetBodyFailsBeyondLimit(t *testing.T) {
	body := NewReplayBody(8)
	reader := body.Reader()
	require.True(t, body.Replayable())

	_, err := body.Write([]byte("12345"))
	require.NoError(t, err)
	require.True(t, body.Replayable())

	_, err = body.Write([]byte("6789"))
	require.NoError(t, err)
	require.False(t, body.Replayable(), "9 bytes exceed an 8-byte limit")

	_, err = reader.GetBody()
	require.ErrorIs(t, err, errReplayExceeded)

	// Streaming is unaffected: the current reader still gets everything.
	require.NoError(t, body.Close())
	all, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "123456789", string(all))
}

func TestReplayBodyTrimsConsumedBytesOnceNotReplayable(t *testing.T) {
	body := NewReplayBody(4)
	reader := body.Reader()

	_, err := body.Write(bytes.Repeat([]byte("a"), 10))
	require.NoError(t, err)
	_, err = io.CopyN(io.Discard, reader, 10)
	require.NoError(t, err)

	body.mu.Lock()
	require.Empty(t, body.buf, "consumed bytes are dropped once replay is impossible")
	require.EqualValues(t, 10, body.base)
	body.mu.Unlock()
}

func TestReplayBodyCloseWithErrorSurfacesToReaders(t *testing.T) {
	body := NewReplayBody(1024)
	reader := body.Reader()
	stdinErr := errors.New("stdin broke")

	_, err := body.Write([]byte("partial"))
	require.NoError(t, err)
	require.NoError(t, body.CloseWithError(stdinErr))

	all, err := io.ReadAll(reader)
	require.ErrorIs(t, err, stdinErr)
	require.Equal(t, "partial", string(all))
}

func TestReplayBodyReaderCloseUnblocksPendingRead(t *testing.T) {
	// The transport closes an abandoned request body while a Read may be
	// waiting for SSH input. That Read must return, or the retry deadlocks.
	body := NewReplayBody(1024)
	reader := body.Reader()

	result := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 8))
		result <- err
	}()

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, reader.Close())

	select {
	case err := <-result:
		require.ErrorIs(t, err, io.ErrClosedPipe)
	case <-time.After(time.Second):
		require.Fail(t, "Read did not return after the reader was closed")
	}

	// Closing one reader does not end the stream for a replay reader.
	_, err := body.Write([]byte("late"))
	require.NoError(t, err)
	require.NoError(t, body.Close())
	replay, err := reader.GetBody()
	require.NoError(t, err)
	all, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, "late", string(all))
}

func TestReplayBodyAbortUnblocksWriterAndReaders(t *testing.T) {
	body := NewReplayBody(1 << 20)
	reader := body.Reader()

	// Fill the window so the writer blocks.
	_, err := body.Write(make([]byte, replayWindow))
	require.NoError(t, err)

	writeResult := make(chan error, 1)
	go func() {
		_, err := body.Write([]byte("x")) // blocks: the reader never drains the window
		writeResult <- err
	}()
	// A second reader on a fresh, empty body blocks waiting for data.
	other := NewReplayBody(1024)
	otherReader := other.Reader()
	readResult := make(chan error, 1)
	go func() {
		_, err := otherReader.Read(make([]byte, 1))
		readResult <- err
	}()

	time.Sleep(20 * time.Millisecond)
	body.Abort()
	other.Abort()
	_ = reader

	for name, ch := range map[string]chan error{"write": writeResult, "read": readResult} {
		select {
		case err := <-ch:
			require.ErrorIs(t, err, io.ErrClosedPipe, name)
		case <-time.After(time.Second):
			require.Fail(t, name+" did not return after Abort")
		}
	}
}

func TestReplayBodyWriterBlocksUntilReaderCatchesUp(t *testing.T) {
	body := NewReplayBody(1 << 20)
	reader := body.Reader()

	_, err := body.Write(make([]byte, replayWindow))
	require.NoError(t, err)

	wrote := make(chan struct{})
	go func() {
		_, _ = body.Write([]byte("y"))
		close(wrote)
	}()

	select {
	case <-wrote:
		require.Fail(t, "Write returned while the reader was replayWindow behind")
	case <-time.After(50 * time.Millisecond):
	}

	_, err = io.CopyN(io.Discard, reader, 1)
	require.NoError(t, err)

	select {
	case <-wrote:
	case <-time.After(time.Second):
		require.Fail(t, "Write did not return after the reader consumed data")
	}
}

func TestReplayBodyWriteAfterCloseFails(t *testing.T) {
	body := NewReplayBody(1024)
	require.NoError(t, body.Close())

	_, err := body.Write([]byte("x"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
}

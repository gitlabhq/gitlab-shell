package pktline

import (
	"bufio"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testNonSpecialPkt = "0008abcd"
	testInvalidPkt    = "invalid packet"
)

var (
	largestString = strings.Repeat("z", 0xffff-4)
)

func TestScanner(t *testing.T) {
	largestPacket := "ffff" + largestString
	testCases := []struct {
		desc string
		in   string
		out  []string
		fail bool
	}{
		{
			desc: "happy path",
			in:   "0010hello world!000000010010hello world!",
			out:  []string{"0010hello world!", "0000", "0001", "0010hello world!"},
		},
		{
			desc: "large input",
			in:   "0010hello world!0000" + largestPacket + "0000",
			out:  []string{"0010hello world!", "0000", largestPacket, "0000"},
		},
		{
			desc: "missing byte middle",
			in:   "0010hello world!00000010010hello world!",
			out:  []string{"0010hello world!", "0000", "0010010hello wor"},
			fail: true,
		},
		{
			desc: "unfinished prefix",
			in:   "0010hello world!000",
			out:  []string{"0010hello world!"},
			fail: true,
		},
		{
			desc: "short read in data, only prefix",
			in:   "0010hello world!0005",
			out:  []string{"0010hello world!"},
			fail: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			scanner := NewScanner(strings.NewReader(tc.in))
			var output []string
			for scanner.Scan() {
				output = append(output, scanner.Text())
			}

			if tc.fail {
				require.Error(t, scanner.Err())
			} else {
				require.NoError(t, scanner.Err())
			}

			require.Equal(t, tc.out, output)
		})
	}
}

func TestIsRefRemoval(t *testing.T) {
	testCases := []struct {
		in        string
		isRemoval bool
	}{
		{in: "003f7217a7c7e582c46cec22a130adf4b9d7d950fba0 7d1665144a3a975c05f1f43902ddaf084e784dbe refs/heads/debug", isRemoval: false},
		{in: "003f0000000000000000000000000000000000000000 7d1665144a3a975c05f1f43902ddaf084e784dbe refs/heads/debug", isRemoval: false},
		{in: "003f7217a7c7e582c46cec22a130adf4b9d7d950fba0 0000000000000000000000000000000000000000 refs/heads/debug", isRemoval: true},
	}

	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.isRemoval, IsRefRemoval([]byte(tc.in)))
		})
	}
}

func TestIsFlush(t *testing.T) {
	testCases := []struct {
		in    string
		flush bool
	}{
		{in: testNonSpecialPkt, flush: false},
		{in: testInvalidPkt, flush: false},
		{in: "0000", flush: true},
	}

	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.flush, IsFlush([]byte(tc.in)))
		})
	}
}

func TestIsReady(t *testing.T) {
	testCases := []struct {
		in    string
		ready bool
	}{
		{in: testNonSpecialPkt, ready: false},
		{in: testInvalidPkt, ready: false},
		{in: "000aready\n", ready: true},
	}

	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.ready, IsReady([]byte(tc.in)))
		})
	}
}

func TestReadPkt(t *testing.T) {
	t.Run("reads a normal packet", func(t *testing.T) {
		r := bufio.NewReader(strings.NewReader("0009done\nrest"))
		pkt, err := ReadPkt(r)
		require.NoError(t, err)
		require.Equal(t, "0009done\n", string(pkt))

		rest, err := io.ReadAll(r)
		require.NoError(t, err)
		require.Equal(t, "rest", string(rest))
	})

	t.Run("reads a special empty packet", func(t *testing.T) {
		r := bufio.NewReader(strings.NewReader("0000rest"))
		pkt, err := ReadPkt(r)
		require.NoError(t, err)
		require.Equal(t, "0000", string(pkt))
	})

	t.Run("EOF with nothing read", func(t *testing.T) {
		r := bufio.NewReader(strings.NewReader(""))
		_, err := ReadPkt(r)
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("invalid length prefix leaves r untouched", func(t *testing.T) {
		r := bufio.NewReader(strings.NewReader("fatal: not a pkt-line at all"))
		_, err := ReadPkt(r)
		require.ErrorIs(t, err, ErrNotPktLine)

		// Nothing was consumed: the caller can still read everything.
		rest, err := io.ReadAll(r)
		require.NoError(t, err)
		require.Equal(t, "fatal: not a pkt-line at all", string(rest))
	})

	t.Run("truncated payload after a valid length prefix is a real error", func(t *testing.T) {
		// Unlike an invalid prefix, once the length is known to be valid the
		// bytes are committed to being a pkt-line - a short payload means the
		// stream itself is broken, so this is not ErrNotPktLine.
		r := bufio.NewReader(strings.NewReader("0009do"))
		_, err := ReadPkt(r)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotPktLine)
	})
}

func TestIsDone(t *testing.T) {
	testCases := []struct {
		in   string
		done bool
	}{
		{in: testNonSpecialPkt, done: false},
		{in: testInvalidPkt, done: false},
		{in: "0009done\n", done: true},
		{in: "0001", done: false},
	}

	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.done, IsDone([]byte(tc.in)))
		})
	}
}

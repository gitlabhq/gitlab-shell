package pktline

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
		{in: "0008abcd", flush: false},
		{in: "invalid packet", flush: false},
		{in: "0000", flush: true},
	}

	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.flush, IsFlush([]byte(tc.in)))
		})
	}
}

func TestIsDone(t *testing.T) {
	testCases := []struct {
		in   string
		done bool
	}{
		{in: "0008abcd", done: false},
		{in: "invalid packet", done: false},
		{in: "0009done\n", done: true},
		{in: "0001", done: false},
	}

	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.done, IsDone([]byte(tc.in)))
		})
	}
}

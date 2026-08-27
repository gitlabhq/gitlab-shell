// Package pktline provides utility functions for working with the Git pkt-line format.
package pktline

// Utility functions for working with the Git pkt-line format. See
// https://github.com/git/git/blob/master/Documentation/technical/protocol-common.txt

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

const (
	maxPktSize = 0xffff
	pktDelim   = "0001"
)

var branchRemovalPktRegexp = regexp.MustCompile(`\A[a-f0-9]{4}[a-f0-9]{40} 0{40} `)

// NewScanner returns a bufio.Scanner that splits on Git pktline boundaries
func NewScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, maxPktSize), maxPktSize)
	scanner.Split(pktLineSplitter)
	return scanner
}

// IsRefRemoval checks if the packet represents a reference removal.
func IsRefRemoval(pkt []byte) bool {
	return branchRemovalPktRegexp.Match(pkt)
}

// IsFlush detects the special flush packet '0000'
func IsFlush(pkt []byte) bool {
	return bytes.Equal(pkt, []byte("0000"))
}

// IsDone detects the special done packet '0009done\n'
func IsDone(pkt []byte) bool {
	return bytes.Equal(pkt, PktDone())
}

// PktDone returns the bytes for a "done" packet.
func PktDone() []byte {
	return []byte("0009done\n")
}

// PktFlush returns the bytes for a flush packet.
func PktFlush() []byte {
	return []byte("0000")
}

// IsReady detects the "ready" packet a protocol v2 fetch response sends once
// negotiation has concluded without the client sending `done`. Only matches
// the plaintext form: if a client ever negotiates the (currently
// unreachable - unadvertised by upload-pack, unenabled in Gitaly/workhorse)
// sideband-all capability, "ready" is sideband-wrapped and this stops
// matching silently.
func IsReady(pkt []byte) bool {
	return bytes.Equal(pkt, PktReady())
}

// PktReady returns the bytes for a "ready" packet.
func PktReady() []byte {
	return []byte("000aready\n")
}

// ErrNotPktLine indicates the next 4 bytes of a ReadPkt caller's reader
// aren't a valid pkt-line length prefix. Unlike other ReadPkt errors, r is
// left untouched, so the caller can fall back to reading it directly (e.g.
// via io.Copy) without losing any bytes - important for forwarding a
// response that turns out not to be pkt-line framed after all, such as a
// plain-text error appended past an already-committed response.
var ErrNotPktLine = errors.New("pktline: not a pkt-line")

// ReadPkt reads and returns exactly one raw pkt-line (the 4-byte hex length
// prefix plus its payload) from r. Unlike NewScanner, this doesn't wrap r in
// its own lookahead buffer, so once pkt-line-aware reading is no longer
// needed the caller can keep reading r directly (e.g. via io.Copy) without
// losing any bytes buffered ahead by a scanner.
func ReadPkt(r *bufio.Reader) ([]byte, error) {
	peeked, err := r.Peek(4)
	if err != nil {
		if len(peeked) > 0 {
			// Fewer than 4 bytes left: too short to be a length prefix, but
			// still real data (e.g. a truncated trailing message). Report
			// ErrNotPktLine so the caller falls back to io.Copy instead of
			// silently dropping it.
			return nil, ErrNotPktLine
		}
		return nil, err
	}
	var prefix [4]byte
	copy(prefix[:], peeked)

	length, parseErr := strconv.ParseInt(string(prefix[:]), 16, 0)
	if parseErr != nil || length < 0 || length > maxPktSize {
		return nil, ErrNotPktLine
	}

	if _, err := r.Discard(4); err != nil {
		return nil, err
	}
	if length < 4 {
		// Special case: magic empty packet 0000, 0001, 0002 or 0003.
		return prefix[:], nil
	}

	pkt := make([]byte, length)
	copy(pkt, prefix[:])
	if _, err := io.ReadFull(r, pkt[4:]); err != nil {
		return nil, err
	}

	return pkt, nil
}

func pktLineSplitter(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if len(data) < 4 {
		if atEOF && len(data) > 0 {
			return 0, nil, fmt.Errorf("pktLineSplitter: incomplete length prefix on %q", data)
		}
		return 0, nil, nil // want more data
	}

	// We have at least 4 bytes available so we can decode the 4-hex digit
	// length prefix of the packet line.
	pktLength64, err := strconv.ParseInt(string(data[:4]), 16, 0)
	if err != nil {
		return 0, nil, fmt.Errorf("pktLineSplitter: decode length: %v", err)
	}

	// Cast is safe because we requested an int-size number from strconv.ParseInt
	pktLength := int(pktLength64)

	if pktLength < 0 {
		return 0, nil, fmt.Errorf("pktLineSplitter: invalid length: %d", pktLength)
	}

	if pktLength < 4 {
		// Special case: magic empty packet 0000, 0001, 0002 or 0003.
		return 4, data[:4], nil
	}

	if len(data) < pktLength {
		// data contains incomplete packet

		if atEOF {
			return 0, nil, fmt.Errorf("pktLineSplitter: less than %d bytes in input %q", pktLength, data)
		}

		return 0, nil, nil // want more data
	}

	return pktLength, data[:pktLength], nil
}

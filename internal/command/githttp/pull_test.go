package githttp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/pktline"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
)

const (
	testInfoRefsPath        = "/info/refs"
	testUnexpectedResponse  = "unexpected response"
	pktDelim                = "0001"
	testAuthorizationHeader = "Authorization"
	testSSHUploadPackPath   = "/ssh-upload-pack"
)

var cloneResponse = `0090want 11d731b83788cd556abea7b465c6bee52d89923c multi_ack_detailed side-band-64k thin-pack ofs-delta deepen-since deepen-not agent=git/2.41.0
0032want e56497bb5f03a90a51293fc6d516788730953899
00000009done
`

// pktLine encodes s as a single Git pkt-line (4-byte hex length prefix + payload).
func pktLine(s string) string {
	return fmt.Sprintf("%04x%s", len(s)+4, s)
}

// lsRefsV2Request is a real protocol v2 ls-refs request (e.g. `git ls-remote`):
// command + capabilities, a delim-pkt, then args terminated by a flush-pkt.
// There's no `done` line, since ls-refs has no negotiation phase.
var lsRefsV2Request = pktLine("command=ls-refs\n") +
	pktLine("agent=git/2.41.0\n") +
	pktDelim +
	pktLine("peel\n") +
	pktLine("symrefs\n") +
	pktLine("ref-prefix HEAD\n") +
	flush

// fetchV2Request is a real protocol v2 fetch request with multi-round `have`
// negotiation: command + capabilities, a delim-pkt, then args ending in `done`
// followed by a trailing flush-pkt.
var fetchV2Request = pktLine("command=fetch\n") +
	pktLine("agent=git/2.41.0\n") +
	pktDelim +
	pktLine("thin-pack\n") +
	pktLine("ofs-delta\n") +
	pktLine("want e56497bb5f03a90a51293fc6d516788730953899\n") +
	pktLine("have 11d731b83788cd556abea7b465c6bee52d89923c\n") +
	pktLine("done\n") +
	flush

func TestPullExecute(t *testing.T) {
	url := setupPull(t, http.StatusOK)
	output := &bytes.Buffer{}
	input := strings.NewReader(cloneResponse)

	cmd := &PullCommand{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{PrimaryRepo: url},
			},
		},
	}

	require.NoError(t, cmd.Execute(context.Background()))
	require.Equal(t, infoRefsWithoutPrefix, output.String())
}

func TestPullExecuteWithSSHUploadPack(t *testing.T) {
	url := setupSSHPull(t, http.StatusOK)
	output := &bytes.Buffer{}
	input := strings.NewReader(cloneResponse)

	cmd := &PullCommand{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{
					PrimaryRepo:                     url,
					GeoProxyFetchSSHDirectToPrimary: true,
					RequestHeaders:                  map[string]string{"Authorization": testGitalyToken},
				},
			},
		},
		Args: &commandargs.Shell{
			Env: sshenv.Env{
				GitProtocolVersion: testGitProtocolVersion,
			},
		},
	}

	require.NoError(t, cmd.Execute(context.Background()))
	require.Equal(t, "upload-pack-response", output.String())
}

// TestPullExecuteWithSSHUploadPackProtocolV2LsRefs covers the multi-command,
// no-`done` case of protocol v2 (e.g. `git ls-remote`): readFromStdin only
// stops on a `done` line, so with none present it should forward the whole
// request, including the trailing flush-pkt, up to EOF.
func TestPullExecuteWithSSHUploadPackProtocolV2LsRefs(t *testing.T) {
	var body string
	requests := []testserver.TestRequestHandler{
		{
			Path: testSSHUploadPackPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				body = string(b)
				w.Write([]byte("upload-pack-response"))
			},
		},
	}
	url := testserver.StartHTTPServer(t, requests)

	output := &bytes.Buffer{}
	input := strings.NewReader(lsRefsV2Request)

	cmd := &PullCommand{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{
					PrimaryRepo:                     url,
					GeoProxyFetchSSHDirectToPrimary: true,
					RequestHeaders:                  map[string]string{testAuthorizationHeader: testGitalyToken},
				},
			},
		},
		Args: &commandargs.Shell{
			Env: sshenv.Env{GitProtocolVersion: testGitProtocolVersion},
		},
	}

	require.NoError(t, cmd.Execute(context.Background()))
	require.Equal(t, lsRefsV2Request, body)
}

// TestPullExecuteWithSSHUploadPackProtocolV2Fetch covers a multi-round `have`
// negotiation under protocol v2. readFromStdin stops once it forwards the
// `done` line and injects the flush-pkt that must terminate a v2 fetch
// request, so the forwarded body matches what real git sends byte for byte.
func TestPullExecuteWithSSHUploadPackProtocolV2Fetch(t *testing.T) {
	var body string
	requests := []testserver.TestRequestHandler{
		{
			Path: testSSHUploadPackPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				body = string(b)
				w.Write([]byte("upload-pack-response"))
			},
		},
	}
	url := testserver.StartHTTPServer(t, requests)

	output := &bytes.Buffer{}
	input := strings.NewReader(fetchV2Request)

	cmd := &PullCommand{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{
					PrimaryRepo:                     url,
					GeoProxyFetchSSHDirectToPrimary: true,
					RequestHeaders:                  map[string]string{testAuthorizationHeader: testGitalyToken},
				},
			},
		},
		Args: &commandargs.Shell{
			Env: sshenv.Env{GitProtocolVersion: testGitProtocolVersion},
		},
	}

	require.NoError(t, cmd.Execute(context.Background()))
	require.Equal(t, fetchV2Request, body)
}

// blockingReader serves data from r, then blocks instead of returning EOF,
// simulating a live SSH client whose stdin stays open (e.g. while it waits
// for a response) instead of actually closing.
type blockingReader struct {
	r       io.Reader
	unblock chan struct{}
}

func (b *blockingReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err == io.EOF { //nolint:errorlint
		<-b.unblock
		return 0, io.EOF
	}
	return n, err
}

// TestPullExecuteWithSSHUploadPackProtocolV2MultiRoundFetch is an end-to-end
// regression guard for the shape that broke the earlier (reverted) attempt
// at this fix: a real v2 fetch can take multiple rounds with no `done` in
// either (round 1 asks for another round via acknowledgments with no
// `ready`; round 2 sends `ready`). Client stdin stays open throughout, like
// a live SSH session, so this also proves requestSSHUploadPack is actually
// wired to copyUploadPackResponse rather than the plain io.Copy path -
// flipping that wiring back off makes this hang/fail instead of passing.
func TestPullExecuteWithSSHUploadPackProtocolV2MultiRoundFetch(t *testing.T) {
	round1 := pktLine("command=fetch\n") +
		pktLine("agent=git/2.41.0\n") +
		pktDelim +
		pktLine("thin-pack\n") +
		pktLine("want e56497bb5f03a90a51293fc6d516788730953899\n") +
		pktLine("have 91811949eef62013b0af985063be777b177d4e57\n") +
		flush // no done: server may ask for another round
	round2 := pktLine("command=fetch\n") +
		pktLine("agent=git/2.41.0\n") +
		pktDelim +
		pktLine("thin-pack\n") +
		pktLine("have 0180c531f914cf94e0a7ff0a83ecd8d3cfdecaac\n") +
		flush // no done here either - matches the real multi-round trace

	round1Response := pktLine("acknowledgments\n") + pktLine("NAK\n") + flush
	round2Response := pktLine("acknowledgments\n") +
		pktLine("ACK 33c7801925509c11319901afd5997368def2571a\n") +
		string(pktline.PktReady()) +
		pktDelim +
		pktLine("packfile\n") +
		"raw pack bytes" +
		flush

	var forwarded bytes.Buffer
	requests := []testserver.TestRequestHandler{
		{
			Path: testSSHUploadPackPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				// This handler runs on its own goroutine, not the test
				// goroutine, so it must use assert (which just records a
				// failure) rather than require (which calls t.FailNow(),
				// documented as only safe from the test goroutine itself).
				br := bufio.NewReader(r.Body)
				readRound := func() {
					for {
						pkt, err := pktline.ReadPkt(br)
						if !assert.NoError(t, err) { //nolint:testifylint // assert, not require: this runs in an HTTP handler goroutine and FailNow is only safe on the test goroutine
							return
						}
						forwarded.Write(pkt)
						if pktline.IsFlush(pkt) {
							return
						}
					}
				}

				readRound()
				_, err := w.Write([]byte(round1Response))
				assert.NoError(t, err)
				assert.NoError(t, http.NewResponseController(w).Flush())

				readRound()
				_, err = w.Write([]byte(round2Response))
				assert.NoError(t, err)
				assert.NoError(t, http.NewResponseController(w).Flush())

				// The request body must close once ready is seen: reading
				// more from it should hit EOF, not block waiting for a round
				// 3 that never comes. If the SSH path weren't wired to
				// copyUploadPackResponse (or closed too late), this blocks
				// and the cmd.Execute timeout below fails the test instead
				// of silently passing.
				_, err = br.ReadByte()
				assert.ErrorIs(t, err, io.EOF)
			},
		},
	}
	url := testserver.StartHTTPServer(t, requests)

	output := &bytes.Buffer{}
	input := &blockingReader{r: strings.NewReader(round1 + round2), unblock: make(chan struct{})}
	defer close(input.unblock)

	cmd := &PullCommand{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{
					PrimaryRepo:                     url,
					GeoProxyFetchSSHDirectToPrimary: true,
					RequestHeaders:                  map[string]string{testAuthorizationHeader: testGitalyToken},
				},
			},
		},
		Args: &commandargs.Shell{
			Env: sshenv.Env{GitProtocolVersion: testGitProtocolVersion},
		},
	}

	execErr := make(chan error, 1)
	go func() {
		execErr <- cmd.Execute(context.Background())
	}()

	select {
	case err := <-execErr:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("cmd.Execute did not return")
	}

	require.Equal(t, round1+round2, forwarded.String())
	require.Equal(t, round1Response+round2Response, output.String())
}

// TestPullCommandCopyUploadPackResponse covers copyUploadPackResponse
// directly rather than through a real HTTP round trip: exercising a genuine
// full-duplex exchange (response bytes arriving while the request is still
// being written) reliably through Go's HTTP client/server in a test would
// need its own synchronization machinery without adding real coverage over
// testing the method directly.
func TestPullCommandCopyUploadPackResponse(t *testing.T) {
	t.Run("closes the request body on ready", func(t *testing.T) {
		response := pktLine("acknowledgments\n") +
			pktLine("ACK 91811949eef62013b0af985063be777b177d4e57\n") +
			string(pktline.PktReady()) +
			pktDelim +
			pktLine("packfile\n") +
			"raw pack bytes" +
			flush

		pipeReader, pipeWriter := io.Pipe()
		defer pipeReader.Close()
		go io.Copy(io.Discard, pipeReader) //nolint:errcheck

		requestClosed := make(chan struct{}) // never closes: no `done`, request still open
		output := &bytes.Buffer{}
		cmd := &PullCommand{ReadWriter: &readwriter.ReadWriter{Out: output}}

		err := cmd.copyUploadPackResponse(pipeWriter, requestClosed, strings.NewReader(response))
		require.NoError(t, err)
		require.Equal(t, response, output.String())

		_, err = pipeWriter.Write([]byte("x"))
		require.ErrorIs(t, err, io.ErrClosedPipe)
	})

	t.Run("skips pkt-line parsing once the request is already closed", func(t *testing.T) {
		requestClosed := make(chan struct{})
		close(requestClosed) // e.g. already closed on `done`

		// Deliberately not pkt-line framed, like raw (non-sideband) pack
		// bytes: parsing this would fail if the already-closed check didn't
		// short-circuit first.
		notPktLineFramed := "this is not pkt-line data, just raw bytes"
		output := &bytes.Buffer{}
		cmd := &PullCommand{ReadWriter: &readwriter.ReadWriter{Out: output}}

		pipeReader, pipeWriter := io.Pipe()
		defer pipeReader.Close()
		go io.Copy(io.Discard, pipeReader) //nolint:errcheck

		err := cmd.copyUploadPackResponse(pipeWriter, requestClosed, strings.NewReader(notPktLineFramed))
		require.NoError(t, err)
		require.Equal(t, notPktLineFramed, output.String())
	})

	// TestPullCommandCopyUploadPackResponse/forwards_a_mid-stream_error covers
	// a response that stops being pkt-line framed partway through: workhorse
	// writes the 200 status before calling Gitaly (ssh.go), so a later
	// failure appends plain text past an already-committed response - the
	// exact shape behind "fatal: protocol error: bad line length character".
	// The error text must still reach the client rather than being replaced
	// by an internal decode error.
	t.Run("forwards a mid-stream error instead of swallowing it", func(t *testing.T) {
		response := pktLine("acknowledgments\n") +
			pktLine("NAK\n") +
			flush + // round 1: no ready, waiting for round 2
			"fatal: internal error, upload-pack died unexpectedly\n"

		requestClosed := make(chan struct{}) // still open: no ready seen yet
		output := &bytes.Buffer{}
		cmd := &PullCommand{ReadWriter: &readwriter.ReadWriter{Out: output}}

		pipeReader, pipeWriter := io.Pipe()
		defer pipeReader.Close()
		go io.Copy(io.Discard, pipeReader) //nolint:errcheck

		err := cmd.copyUploadPackResponse(pipeWriter, requestClosed, strings.NewReader(response))
		require.NoError(t, err)
		require.Equal(t, response, output.String())
	})

	// A trailing 1-3 byte tail is too short to even be a length prefix, so
	// bufio.Reader.Peek(4) returns it alongside io.EOF - the same byte-loss
	// shape as the mid-stream error case above, just at true EOF instead of
	// a decode failure.
	t.Run("forwards a trailing tail shorter than a length prefix", func(t *testing.T) {
		response := pktLine("acknowledgments\n") +
			pktLine("NAK\n") +
			flush +
			"xy" // 2 bytes: not enough for even a length prefix

		requestClosed := make(chan struct{})
		output := &bytes.Buffer{}
		cmd := &PullCommand{ReadWriter: &readwriter.ReadWriter{Out: output}}

		pipeReader, pipeWriter := io.Pipe()
		defer pipeReader.Close()
		go io.Copy(io.Discard, pipeReader) //nolint:errcheck

		err := cmd.copyUploadPackResponse(pipeWriter, requestClosed, strings.NewReader(response))
		require.NoError(t, err)
		require.Equal(t, response, output.String())
	})

	// TestPullCommandCopyUploadPackResponse/multi-round_negotiation is the
	// regression guard for the case that broke the earlier attempt at this
	// fix: a real v2 fetch can take multiple rounds with no `done` in either
	// (round 1 acks without `ready`, asking for more `have`s; round 2 acks
	// with `ready`). It checks not just the final bytes but the actual
	// timing: the request body must still accept a real client's round-2
	// write in the gap between round 1 and round 2, and only close once
	// `ready` arrives - closing any earlier would strand a real client mid
	// negotiation, exactly what the earlier (reverted) attempt at this fix
	// did.
	t.Run("multi-round negotiation: stays open until the round with ready", func(t *testing.T) {
		responseReader, responseWriter := io.Pipe()
		defer responseWriter.Close() //nolint:errcheck

		pipeReader, pipeWriter := io.Pipe()
		defer pipeReader.Close()
		drained := make(chan []byte, 1)
		go func() {
			b, _ := io.ReadAll(pipeReader)
			drained <- b
		}()

		requestClosed := make(chan struct{})
		output := &bytes.Buffer{}
		cmd := &PullCommand{ReadWriter: &readwriter.ReadWriter{Out: output}}

		copyDone := make(chan error, 1)
		go func() {
			copyDone <- cmd.copyUploadPackResponse(pipeWriter, requestClosed, responseReader)
		}()

		round1 := pktLine("acknowledgments\n") +
			pktLine("NAK\n") +
			flush // no ready: server wants another round of `have`s
		_, err := responseWriter.Write([]byte(round1))
		require.NoError(t, err)

		// Give any incorrect "close early" logic every chance to run before
		// checking: a wrong implementation closes synchronously right after
		// processing round 1's flush, so this settle window reliably tells
		// "already closed (wrong)" apart from "genuinely still waiting
		// (right)" - round 2 is deliberately not sent yet.
		time.Sleep(100 * time.Millisecond)
		select {
		case copyErr := <-copyDone:
			t.Fatalf("copyUploadPackResponse returned before ready (err=%v); it must wait for more rounds", copyErr)
		default:
		}

		// Round 1 carried no `ready`, so a real client would still be able to
		// send round 2's `have`s on this same request body right now.
		round2Have := "round 2 have\n"
		writeErr := make(chan error, 1)
		go func() {
			_, sendErr := pipeWriter.Write([]byte(round2Have))
			writeErr <- sendErr
		}()
		select {
		case sendErr := <-writeErr:
			require.NoError(t, sendErr, "request body closed before round 2's ready; round 2 could never be sent")
		case <-time.After(2 * time.Second):
			t.Fatal("write of round 2 blocked - reader side may be gone")
		}

		round2 := pktLine("acknowledgments\n") +
			pktLine("ACK 33c7801925509c11319901afd5997368def2571a\n") +
			string(pktline.PktReady()) +
			pktDelim +
			pktLine("packfile\n") +
			"raw pack bytes" +
			flush
		_, err = responseWriter.Write([]byte(round2))
		require.NoError(t, err)
		require.NoError(t, responseWriter.Close())

		select {
		case copyErr := <-copyDone:
			require.NoError(t, copyErr)
		case <-time.After(2 * time.Second):
			t.Fatal("copyUploadPackResponse did not return after ready")
		}
		require.Equal(t, round1+round2, output.String())
		require.Equal(t, round2Have, string(<-drained))

		_, err = pipeWriter.Write([]byte("x"))
		require.ErrorIs(t, err, io.ErrClosedPipe)
	})
}

// TestReadFromStdinStopsOnClosedPipe covers readFromStdin's reaction to
// copyUploadPackResponse closing the request body first (on `ready`): the
// next write must stop the loop instead of logging one error per remaining
// packet, and requestClosed must still be signaled.
func TestReadFromStdinStopsOnClosedPipe(t *testing.T) {
	pipeReader, pipeWriter := io.Pipe()
	defer pipeReader.Close()
	require.NoError(t, pipeWriter.Close())

	input := strings.NewReader(pktLine("have aaaa\n") + pktLine("have bbbb\n") + pktLine("have cccc\n"))
	cmd := &PullCommand{ReadWriter: &readwriter.ReadWriter{In: input}}
	requestClosed := make(chan struct{})

	done := make(chan struct{})
	go func() {
		cmd.readFromStdin(pipeWriter, requestClosed)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readFromStdin did not return after its writes started failing")
	}

	select {
	case <-requestClosed:
	default:
		t.Fatal("requestClosed was not signaled")
	}
}

// TestReadFromStdinPropagatesScannerError covers stdin truncated mid-pkt-line:
// the request body must surface that as an error rather than a clean-looking
// EOF, which would make the primary treat a broken request as complete.
func TestReadFromStdinPropagatesScannerError(t *testing.T) {
	input := strings.NewReader("0009ab") // claims 5 bytes of payload, only 2 follow
	cmd := &PullCommand{ReadWriter: &readwriter.ReadWriter{In: input}}

	pipeReader, pipeWriter := io.Pipe()
	requestClosed := make(chan struct{})

	done := make(chan struct{})
	go func() {
		cmd.readFromStdin(pipeWriter, requestClosed)
		close(done)
	}()

	_, readErr := io.ReadAll(pipeReader)
	require.Error(t, readErr)
	require.NotErrorIs(t, readErr, io.EOF)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readFromStdin did not return")
	}
}

func TestPullExecuteWithFailedInfoRefs(t *testing.T) {
	testCases := []struct {
		desc            string
		statusCode      int
		responseContent string
		expectedErr     string
	}{
		{
			desc:        "request failed",
			statusCode:  http.StatusForbidden,
			expectedErr: "Remote repository is unavailable",
		}, {
			desc:            "unexpected response",
			statusCode:      http.StatusOK,
			responseContent: testUnexpectedResponse,
			expectedErr:     "unexpected git-upload-pack response",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			requests := []testserver.TestRequestHandler{
				{
					Path: testInfoRefsPath,
					Handler: func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, "git-upload-pack", r.URL.Query().Get("service"))

						w.WriteHeader(tc.statusCode)
						w.Write([]byte(tc.responseContent))
					},
				},
			}

			url := testserver.StartHTTPServer(t, requests)

			cmd := &PullCommand{
				Config: &config.Config{GitlabURL: url},
				Response: &accessverifier.Response{
					Payload: accessverifier.CustomPayload{
						Data: accessverifier.CustomPayloadData{PrimaryRepo: url},
					},
				},
			}

			err := cmd.Execute(context.Background())
			require.Error(t, err)
			require.Equal(t, tc.expectedErr, err.Error())
		})
	}
}

func TestExecuteWithFailedUploadPack(t *testing.T) {
	url := setupPull(t, http.StatusForbidden)
	output := &bytes.Buffer{}
	input := strings.NewReader(cloneResponse)

	cmd := &PullCommand{
		Config:     &config.Config{GitlabURL: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{PrimaryRepo: url},
			},
		},
	}

	err := cmd.Execute(context.Background())
	require.Error(t, err)
	require.Equal(t, "Remote repository is unavailable", err.Error())
}

func setupPull(t *testing.T, uploadPackStatusCode int) string {
	infoRefs := "001e# service=git-upload-pack\n" + flush + infoRefsWithoutPrefix

	requests := []testserver.TestRequestHandler{
		{
			Path: testInfoRefsPath,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "git-upload-pack", r.URL.Query().Get("service"))

				w.Write([]byte(infoRefs))
			},
		},
		{
			Path: "/git-upload-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				assert.True(t, strings.HasSuffix(string(body), "0009done\n"+flush))

				w.WriteHeader(uploadPackStatusCode)
			},
		},
	}

	return testserver.StartHTTPServer(t, requests)
}

func setupSSHPull(t *testing.T, uploadPackStatusCode int) string {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/ssh-upload-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				assert.True(t, strings.HasSuffix(string(body), "0009done\n"+flush))
				assert.Equal(t, testGitProtocolVersion, r.Header.Get("Git-Protocol"))
				assert.Equal(t, testGitalyToken, r.Header.Get("Authorization"))

				w.Write([]byte("upload-pack-response"))
				w.WriteHeader(uploadPackStatusCode)
			},
		},
	}

	return testserver.StartHTTPServer(t, requests)
}

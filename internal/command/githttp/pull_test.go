package githttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
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

// TestPullExecuteWithSSHUploadPackProtocolV2 covers protocol v2 requests.
// readUploadPackRequest only stops on a `done` line, so ls-refs (which has none)
// forwards up to EOF as-is, while fetch's multi-round `have` negotiation ends
// with readUploadPackRequest forwarding `done` and injecting the trailing flush-pkt
// that must terminate the request. Either way, the forwarded body should
// match what real git sends byte for byte.
func TestPullExecuteWithSSHUploadPackProtocolV2(t *testing.T) {
	testCases := []struct {
		desc    string
		request string
	}{
		{desc: "ls-refs (no done)", request: lsRefsV2Request},
		{desc: "fetch (multi-round have negotiation)", request: fetchV2Request},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
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
			input := strings.NewReader(tc.request)

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
			require.Equal(t, tc.request, body)
		})
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

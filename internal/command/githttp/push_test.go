package githttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
)

var (
	flush                 = "0000"
	infoRefsWithoutPrefix = "00c4e56497bb5f03a90a51293fc6d516788730953899 refs/heads/'test'report-status " +
		"report-status-v2 delete-refs side-band-64k quiet atomic ofs-delta push-options object-format=sha1 " +
		"agent=git/2.38.3.gl200\n" + flush
)

func TestExecute(t *testing.T) {
	url, input := setup(t, http.StatusOK)
	output := &bytes.Buffer{}

	cmd := &PushCommand{
		Config:     &config.Config{GitlabUrl: url},
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

func TestExecuteWithFailedInfoRefs(t *testing.T) {
	testCases := []struct {
		desc            string
		statusCode      int
		responseContent string
		expectedErr     string
	}{
		{
			desc:        "request failed",
			statusCode:  http.StatusForbidden,
			expectedErr: "Internal API error (403)",
		}, {
			desc:            "unexpected response",
			statusCode:      http.StatusOK,
			responseContent: "unexpected response",
			expectedErr:     "Unexpected git-receive-pack response",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			requests := []testserver.TestRequestHandler{
				{
					Path: "/info/refs",
					Handler: func(w http.ResponseWriter, r *http.Request) {
						require.Equal(t, "git-receive-pack", r.URL.Query().Get("service"))

						w.WriteHeader(tc.statusCode)
						w.Write([]byte(tc.responseContent))
					},
				},
			}

			url := testserver.StartHttpServer(t, requests)

			cmd := &PushCommand{
				Config: &config.Config{GitlabUrl: url},
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

func TestExecuteWithFailedReceivePack(t *testing.T) {
	url, input := setup(t, http.StatusForbidden)
	output := &bytes.Buffer{}

	cmd := &PushCommand{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{PrimaryRepo: url},
			},
		},
	}

	err := cmd.Execute(context.Background())
	require.Error(t, err)
	require.Equal(t, "Internal API error (403)", err.Error())
}

func setup(t *testing.T, receivePackStatusCode int) (string, io.Reader) {
	infoRefs := "001f# service=git-receive-pack\n" + flush + infoRefsWithoutPrefix
	receivePackPrefix := "00ab4c9d98d7750fa65db8ddcc60a89ef919f7a179f9 df505c066e4e63a801268a84627d7e8f7e033c7a " +
		"refs/heads/main123 report-status-v2 side-band-64k object-format=sha1 agent=git/2.39.1"
	receivePackData := "PACK some data"

	// Imitate sending data via multiple packets
	input := io.MultiReader(
		strings.NewReader(receivePackPrefix),
		strings.NewReader(flush),
		strings.NewReader(receivePackData),
	)

	requests := []testserver.TestRequestHandler{
		{
			Path: "/info/refs",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "git-receive-pack", r.URL.Query().Get("service"))

				w.Write([]byte(infoRefs))
			},
		},
		{
			Path: "/git-receive-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				defer r.Body.Close()

				require.Equal(t, receivePackPrefix+flush+receivePackData, string(body))
				w.WriteHeader(receivePackStatusCode)
			},
		},
	}

	return testserver.StartHttpServer(t, requests), input
}

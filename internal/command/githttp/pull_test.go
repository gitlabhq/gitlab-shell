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

func TestPullExecute(t *testing.T) {
	url := setupPull(t, http.StatusOK)
	output := &bytes.Buffer{}

	cmd := &PullCommand{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: nil},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{PrimaryRepo: url},
			},
		},
	}

	require.NoError(t, cmd.Execute(context.Background()))
	require.Equal(t, infoRefsWithoutPrefix, output.String())
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
			responseContent: "unexpected response",
			expectedErr:     "Unexpected git-upload-pack response",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			requests := []testserver.TestRequestHandler{
				{
					Path: "/info/refs",
					Handler: func(w http.ResponseWriter, r *http.Request) {
						require.Equal(t, "git-upload-pack", r.URL.Query().Get("service"))

						w.WriteHeader(tc.statusCode)
						w.Write([]byte(tc.responseContent))
					},
				},
			}

			url := testserver.StartHttpServer(t, requests)

			cmd := &PullCommand{
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

func TestExecuteWithFailedUploadPack(t *testing.T) {
	url := setupPull(t, http.StatusForbidden)
	output := &bytes.Buffer{}

	cmd := &PullCommand{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: nil},
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
	infoRefs := "001f# service=git-upload-pack\n" + flush + infoRefsWithoutPrefix
	uploadPackPrefix := "00ab4c9d98d7750fa65db8ddcc60a89ef919f7a179f9 df505c066e4e63a801268a84627d7e8f7e033c7a " +
		"refs/heads/main123 report-status-v2 side-band-64k object-format=sha1 agent=git/2.39.1"
	uploadPackData := "PACK some uploaded data"

	// Imitate response data via multiple packets
	response := io.MultiReader(
		strings.NewReader(uploadPackPrefix),
		strings.NewReader(flush),
		strings.NewReader(uploadPackData),
	)

	requests := []testserver.TestRequestHandler{
		{
			Path: "/info/refs",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "git-upload-pack", r.URL.Query().Get("service"))

				w.Write([]byte(infoRefs))
			},
		},
		{
			Path: "/git-upload-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(response)
				require.NoError(t, err)
				defer r.Body.Close()

				require.Equal(t, uploadPackPrefix+flush+uploadPackData, string(data))
				w.WriteHeader(uploadPackStatusCode)
			},
		},
	}

	return testserver.StartHttpServer(t, requests)
}

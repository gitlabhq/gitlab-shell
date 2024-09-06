package githttp

import (
	"bytes"
	"context"
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
	assert.Equal(t, infoRefsWithoutPrefix, output.String())
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
			expectedErr: "Remote repository is unavailable",
		}, {
			desc:            "request failed with body",
			statusCode:      http.StatusForbidden,
			responseContent: "You are not allowed to push code to this project",
			expectedErr:     "You are not allowed to push code to this project",
		}, {
			desc:            "unexpected response",
			statusCode:      http.StatusOK,
			responseContent: "unexpected response",
			expectedErr:     "unexpected git-receive-pack response",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			requests := []testserver.TestRequestHandler{
				{
					Path: "/info/refs",
					Handler: func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, "git-receive-pack", r.URL.Query().Get("service"))

						w.WriteHeader(tc.statusCode)
						w.Write([]byte(tc.responseContent))
					},
				},
			}

			url := testserver.StartHTTPServer(t, requests)

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
			assert.Equal(t, tc.expectedErr, err.Error())
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
	assert.Equal(t, "Remote repository is unavailable", err.Error())
}

func TestPushExecuteWithSSHReceivePack(t *testing.T) {
	url := setupSSHPush(t, http.StatusOK)
	output := &bytes.Buffer{}
	input := strings.NewReader(cloneResponse + "0009done\n")

	cmd := &PushCommand{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{Out: output, In: input},
		Response: &accessverifier.Response{
			Payload: accessverifier.CustomPayload{
				Data: accessverifier.CustomPayloadData{
					PrimaryRepo:                    url,
					GeoProxyDirectToPrimary:        true,
					GeoProxyPushSSHDirectToPrimary: true,
					RequestHeaders:                 map[string]string{"Authorization": "token"},
				},
			},
		},
		Args: &commandargs.Shell{
			Env: sshenv.Env{
				GitProtocolVersion: "version=2",
			},
		},
	}

	require.NoError(t, cmd.Execute(context.Background()))
	assert.Equal(t, "receive-pack-response", output.String())
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
				assert.Equal(t, "git-receive-pack", r.URL.Query().Get("service"))

				w.Write([]byte(infoRefs))
			},
		},
		{
			Path: "/git-receive-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				assert.Equal(t, receivePackPrefix+flush+receivePackData, string(body))
				w.WriteHeader(receivePackStatusCode)
			},
		},
	}

	return testserver.StartHTTPServer(t, requests), input
}

func setupSSHPush(t *testing.T, uploadPackStatusCode int) string {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/ssh-receive-pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				defer r.Body.Close()

				assert.True(t, strings.HasSuffix(string(body), "0009done\n"))
				assert.Equal(t, "version=2", r.Header.Get("Git-Protocol"))
				assert.Equal(t, "token", r.Header.Get("Authorization"))

				w.WriteHeader(uploadPackStatusCode)
				w.Write([]byte("receive-pack-response"))
			},
		},
	}

	return testserver.StartHTTPServer(t, requests)
}

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

var cloneResponse = `0090want 11d731b83788cd556abea7b465c6bee52d89923c multi_ack_detailed side-band-64k thin-pack ofs-delta deepen-since deepen-not agent=git/2.41.0
0032want e56497bb5f03a90a51293fc6d516788730953899
00000009done
`

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
					RequestHeaders:                  map[string]string{"Authorization": "token"},
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
	require.Equal(t, "upload-pack-response", output.String())
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
			expectedErr:     "unexpected git-upload-pack response",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			requests := []testserver.TestRequestHandler{
				{
					Path: "/info/refs",
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
			Path: "/info/refs",
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

				assert.True(t, strings.HasSuffix(string(body), "0009done\n"))

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

				assert.True(t, strings.HasSuffix(string(body), "0009done\n"))
				assert.Equal(t, "version=2", r.Header.Get("Git-Protocol"))
				assert.Equal(t, "token", r.Header.Get("Authorization"))

				w.Write([]byte("upload-pack-response"))
				w.WriteHeader(uploadPackStatusCode)
			},
		},
	}

	return testserver.StartHTTPServer(t, requests)
}

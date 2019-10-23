package accessverifier

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/accessverifier"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
)

var (
	repo   = "group/repo"
	action = commandargs.ReceivePack
)

func setup(t *testing.T) (*Command, *bytes.Buffer, *bytes.Buffer, func()) {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				var requestBody *accessverifier.Request
				err = json.Unmarshal(b, &requestBody)
				require.NoError(t, err)

				if requestBody.KeyId == "1" {
					body := map[string]interface{}{
						"gl_console_messages": []string{"console", "message"},
					}
					require.NoError(t, json.NewEncoder(w).Encode(body))
				} else {
					body := map[string]interface{}{
						"status":  false,
						"message": "missing user",
					}
					require.NoError(t, json.NewEncoder(w).Encode(body))
				}
			},
		},
	}

	url, cleanup := testserver.StartSocketHttpServer(t, requests)

	errBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}

	readWriter := &readwriter.ReadWriter{Out: outBuf, ErrOut: errBuf}
	cmd := &Command{Config: &config.Config{GitlabUrl: url}, ReadWriter: readWriter}

	return cmd, errBuf, outBuf, cleanup
}

func TestMissingUser(t *testing.T) {
	cmd, _, _, cleanup := setup(t)
	defer cleanup()

	cmd.Args = &commandargs.Shell{GitlabKeyId: "2"}
	_, err := cmd.Verify(action, repo)

	require.Equal(t, "missing user", err.Error())
}

func TestConsoleMessages(t *testing.T) {
	cmd, errBuf, outBuf, cleanup := setup(t)
	defer cleanup()

	cmd.Args = &commandargs.Shell{GitlabKeyId: "1"}
	cmd.Verify(action, repo)

	require.Equal(t, "remote: \nremote: console\nremote: message\nremote: \n", errBuf.String())
	require.Empty(t, outBuf.String())
}

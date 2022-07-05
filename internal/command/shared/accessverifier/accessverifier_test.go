package accessverifier

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
)

var (
	repo   = "group/repo"
	action = commandargs.ReceivePack
)

func setup(t *testing.T) (*Command, *bytes.Buffer, *bytes.Buffer) {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
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

	url := testserver.StartSocketHttpServer(t, requests)

	errBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}

	readWriter := &readwriter.ReadWriter{Out: outBuf, ErrOut: errBuf}
	cmd := &Command{Config: &config.Config{GitlabUrl: url}, ReadWriter: readWriter}

	return cmd, errBuf, outBuf
}

func TestMissingUser(t *testing.T) {
	cmd, _, _ := setup(t)

	cmd.Args = &commandargs.Shell{GitlabKeyId: "2"}
	_, err := cmd.Verify(context.Background(), action, repo)

	require.Equal(t, "missing user", err.Error())
}

func TestConsoleMessages(t *testing.T) {
	cmd, errBuf, outBuf := setup(t)

	cmd.Args = &commandargs.Shell{GitlabKeyId: "1"}
	cmd.Verify(context.Background(), action, repo)

	require.Equal(t, "remote: \nremote: console\nremote: message\nremote: \n", errBuf.String())
	require.Empty(t, outBuf.String())
}

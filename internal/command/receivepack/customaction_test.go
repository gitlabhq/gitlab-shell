package receivepack

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

func TestCustomReceivePack(t *testing.T) {
	repo := "group/repo"
	keyId := "1"

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				var request *accessverifier.Request
				require.NoError(t, json.Unmarshal(b, &request))

				require.Equal(t, "1", request.KeyId)

				body := map[string]interface{}{
					"status": true,
					"gl_id":  "1",
					"payload": map[string]interface{}{
						"action": "geo_proxy_to_primary",
						"data": map[string]interface{}{
							"api_endpoints": []string{"/geo/proxy_git_push_ssh/info_refs", "/geo/proxy_git_push_ssh/push"},
							"gl_username":   "custom",
							"primary_repo":  "https://repo/path",
						},
					},
				}
				w.WriteHeader(http.StatusMultipleChoices)
				require.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
		{
			Path: "/geo/proxy_git_push_ssh/info_refs",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				var request *Request
				require.NoError(t, json.Unmarshal(b, &request))

				require.Equal(t, request.Data.UserId, "key-"+keyId)
				require.Empty(t, request.Output)

				err = json.NewEncoder(w).Encode(Response{Result: []byte("custom")})
				require.NoError(t, err)
			},
		},
		{
			Path: "/geo/proxy_git_push_ssh/push",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				var request *Request
				require.NoError(t, json.Unmarshal(b, &request))

				require.Equal(t, request.Data.UserId, "key-"+keyId)
				require.Equal(t, "input", string(request.Output))

				err = json.NewEncoder(w).Encode(Response{Result: []byte("output")})
				require.NoError(t, err)
			},
		},
	}

	url, cleanup := testserver.StartSocketHttpServer(t, requests)
	defer cleanup()

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	input := bytes.NewBufferString("input")

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		Args:       &commandargs.Shell{GitlabKeyId: keyId, CommandType: commandargs.ReceivePack, SshArgs: []string{"git-receive-pack", repo}},
		ReadWriter: &readwriter.ReadWriter{ErrOut: errBuf, Out: outBuf, In: input},
	}

	require.NoError(t, cmd.Execute())

	// expect printing of info message, "custom" string from the first request
	// and "output" string from the second request
	require.Equal(t, "customoutput", outBuf.String())
}

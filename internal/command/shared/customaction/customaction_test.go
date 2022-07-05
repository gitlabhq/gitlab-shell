package customaction

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/readwriter"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/accessverifier"
)

func TestExecuteEOFSent(t *testing.T) {
	who := "key-1"

	requests := []testserver.TestRequestHandler{
		{
			Path: "/geo/proxy/info_refs_receive_pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var request *Request
				require.NoError(t, json.Unmarshal(b, &request))

				require.Equal(t, request.Data.UserId, who)
				require.Empty(t, request.Output)

				err = json.NewEncoder(w).Encode(Response{Result: []byte("custom")})
				require.NoError(t, err)
			},
		},
		{
			Path: "/geo/proxy/receive_pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var request *Request
				require.NoError(t, json.Unmarshal(b, &request))

				require.Equal(t, request.Data.UserId, who)
				require.Equal(t, "0009input", string(request.Output))

				err = json.NewEncoder(w).Encode(Response{Result: []byte("output")})
				require.NoError(t, err)
			},
		},
	}

	url := testserver.StartSocketHttpServer(t, requests)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	input := bytes.NewBufferString("0009input")

	response := &accessverifier.Response{
		Who: who,
		Payload: accessverifier.CustomPayload{
			Action: "geo_proxy_to_primary",
			Data: accessverifier.CustomPayloadData{
				ApiEndpoints: []string{"/geo/proxy/info_refs_receive_pack", "/geo/proxy/receive_pack"},
				Username:     "custom",
				PrimaryRepo:  "https://repo/path",
			},
		},
	}

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{ErrOut: errBuf, Out: outBuf, In: input},
		EOFSent:    true,
	}

	require.NoError(t, cmd.Execute(context.Background(), response))

	// expect printing of info message, "custom" string from the first request
	// and "output" string from the second request
	require.Equal(t, "customoutput", outBuf.String())
}

func TestExecuteNoEOFSent(t *testing.T) {
	who := "key-1"

	requests := []testserver.TestRequestHandler{
		{
			Path: "/geo/proxy/info_refs_upload_pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var request *Request
				require.NoError(t, json.Unmarshal(b, &request))

				require.Equal(t, request.Data.UserId, who)
				require.Empty(t, request.Output)

				err = json.NewEncoder(w).Encode(Response{Result: []byte("custom")})
				require.NoError(t, err)
			},
		},
		{
			Path: "/geo/proxy/upload_pack",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var request *Request
				require.NoError(t, json.Unmarshal(b, &request))

				require.Equal(t, request.Data.UserId, who)
				require.Equal(t, "0032want 343d70886785dc1f98aaf70f3b4ca87c93a5d0dd\n", string(request.Output))

				err = json.NewEncoder(w).Encode(Response{Result: []byte("output")})
				require.NoError(t, err)
			},
		},
	}

	url := testserver.StartSocketHttpServer(t, requests)

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	input := bytes.NewBufferString("0032want 343d70886785dc1f98aaf70f3b4ca87c93a5d0dd\n")

	response := &accessverifier.Response{
		Who: who,
		Payload: accessverifier.CustomPayload{
			Action: "geo_proxy_to_primary",
			Data: accessverifier.CustomPayloadData{
				ApiEndpoints: []string{"/geo/proxy/info_refs_upload_pack", "/geo/proxy/upload_pack"},
				Username:     "custom",
				PrimaryRepo:  "https://repo/path",
			},
		},
	}

	cmd := &Command{
		Config:     &config.Config{GitlabUrl: url},
		ReadWriter: &readwriter.ReadWriter{ErrOut: errBuf, Out: outBuf, In: input},
		EOFSent:    false,
	}

	require.NoError(t, cmd.Execute(context.Background(), response))

	// expect printing of info message, "custom" string from the first request
	// and "output" string from the second request
	require.Equal(t, "customoutput", outBuf.String())
}

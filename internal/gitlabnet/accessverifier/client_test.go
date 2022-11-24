package accessverifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly/v15/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/sshenv"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"
)

var (
	repo              = "group/private"
	receivePackAction = commandargs.ReceivePack
	uploadPackAction  = commandargs.UploadPack
)

func buildExpectedResponse(who string) *Response {
	response := &Response{
		Success:          true,
		UserId:           "user-1",
		Repo:             "project-26",
		Username:         "root",
		GitConfigOptions: []string{"option"},
		Gitaly: Gitaly{
			Repo: pb.Repository{
				StorageName:                   "default",
				RelativePath:                  "@hashed/5f/9c/5f9c4ab08cac7457e9111a30e4664920607ea2c115a1433d7be98e97e64244ca.git",
				GitObjectDirectory:            "path/to/git_object_directory",
				GitAlternateObjectDirectories: []string{"path/to/git_alternate_object_directory"},
				GlRepository:                  "project-26",
				GlProjectPath:                 repo,
			},
			Address: "unix:gitaly.socket",
			Token:   "token",
		},
		GitProtocol:     "protocol",
		Payload:         CustomPayload{},
		ConsoleMessages: []string{"console", "message"},
		Who:             who,
		StatusCode:      200,
	}

	return response
}

func TestSuccessfulResponses(t *testing.T) {
	okResponse := testResponse{body: responseBody(t, "allowed.json"), status: http.StatusOK}
	client := setup(t,
		map[string]testResponse{"first": okResponse, "test@TEST.TEST": okResponse},
		map[string]testResponse{"1": okResponse},
	)

	testCases := []struct {
		desc string
		args *commandargs.Shell
		who  string
	}{
		{
			desc: "Provide key id within the request",
			args: &commandargs.Shell{GitlabKeyId: "1"},
			who:  "key-1",
		}, {
			desc: "Provide username within the request",
			args: &commandargs.Shell{GitlabUsername: "first"},
			who:  "user-1",
		}, {
			desc: "Provide krb5principal within the request",
			args: &commandargs.Shell{GitlabKrb5Principal: "test@TEST.TEST"},
			who:  "user-1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := client.Verify(context.Background(), tc.args, receivePackAction, repo)
			require.NoError(t, err)

			response := buildExpectedResponse(tc.who)
			require.Equal(t, response, result)
		})
	}
}

func TestGeoPushGetCustomAction(t *testing.T) {
	client := setup(t, map[string]testResponse{
		"custom": {
			body:   responseBody(t, "allowed_with_push_payload.json"),
			status: 300,
		},
	}, nil)

	args := &commandargs.Shell{GitlabUsername: "custom"}
	result, err := client.Verify(context.Background(), args, receivePackAction, repo)
	require.NoError(t, err)

	response := buildExpectedResponse("user-1")
	response.Payload = CustomPayload{
		Action: "geo_proxy_to_primary",
		Data: CustomPayloadData{
			ApiEndpoints: []string{"geo/proxy_git_ssh/info_refs_receive_pack", "geo/proxy_git_ssh/receive_pack"},
			Username:     "custom",
			PrimaryRepo:  "https://repo/path",
		},
	}
	response.StatusCode = 300

	require.True(t, response.IsCustomAction())
	require.Equal(t, response, result)
}

func TestGeoPullGetCustomAction(t *testing.T) {
	client := setup(t, map[string]testResponse{
		"custom": {
			body:   responseBody(t, "allowed_with_pull_payload.json"),
			status: 300,
		},
	}, nil)

	args := &commandargs.Shell{GitlabUsername: "custom"}
	result, err := client.Verify(context.Background(), args, uploadPackAction, repo)
	require.NoError(t, err)

	response := buildExpectedResponse("user-1")
	response.Payload = CustomPayload{
		Action: "geo_proxy_to_primary",
		Data: CustomPayloadData{
			ApiEndpoints: []string{"geo/proxy_git_ssh/info_refs_upload_pack", "geo/proxy_git_ssh/upload_pack"},
			Username:     "custom",
			PrimaryRepo:  "https://repo/path",
		},
	}
	response.StatusCode = 300

	require.True(t, response.IsCustomAction())
	require.Equal(t, response, result)
}

func TestErrorResponses(t *testing.T) {
	client := setup(t, nil, map[string]testResponse{
		"2": {body: []byte(`{"message":"Not allowed!"}`), status: http.StatusForbidden},
		"3": {body: []byte(`{"message":"broken json!`), status: http.StatusOK},
		"4": {status: http.StatusForbidden},
	})

	testCases := []struct {
		desc          string
		fakeId        string
		expectedError string
	}{
		{
			desc:          "A response with an error message",
			fakeId:        "2",
			expectedError: "Not allowed!",
		},
		{
			desc:          "A response with bad JSON",
			fakeId:        "3",
			expectedError: "Parsing failed",
		},
		{
			desc:          "An error response without message",
			fakeId:        "4",
			expectedError: "Internal API error (403)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			args := &commandargs.Shell{GitlabKeyId: tc.fakeId}
			resp, err := client.Verify(context.Background(), args, receivePackAction, repo)

			require.EqualError(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

func TestCheckIP(t *testing.T) {
	testCases := []struct {
		desc            string
		remoteAddr      string
		expectedCheckIp string
	}{
		{
			desc:            "IPv4 address",
			remoteAddr:      "18.245.0.42",
			expectedCheckIp: "18.245.0.42",
		},
		{
			desc:            "IPv6 address",
			remoteAddr:      "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expectedCheckIp: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		},
		{
			desc:            "Host and port",
			remoteAddr:      "18.245.0.42:6345",
			expectedCheckIp: "18.245.0.42",
		},
		{
			desc:            "IPv6 host and port",
			remoteAddr:      "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:80",
			expectedCheckIp: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		},
		{
			desc:            "Bad remote addr",
			remoteAddr:      "[127.0",
			expectedCheckIp: "[127.0",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client := setupWithApiInspector(t,
				func(r *Request) {
					require.Equal(t, tc.expectedCheckIp, r.CheckIp)
				})

			sshEnv := sshenv.Env{RemoteAddr: tc.remoteAddr}
			client.Verify(context.Background(), &commandargs.Shell{Env: sshEnv}, uploadPackAction, repo)
		})
	}
}

type testResponse struct {
	body   []byte
	status int
}

func responseBody(t *testing.T, name string) []byte {
	t.Helper()
	testhelper.PrepareTestRootDir(t)
	body, err := os.ReadFile(path.Join(testhelper.TestRoot, "responses", name))
	require.NoError(t, err)
	return body
}

func setup(t *testing.T, userResponses, keyResponses map[string]testResponse) *Client {
	t.Helper()
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var requestBody *Request
				require.NoError(t, json.Unmarshal(b, &requestBody))

				if tr, ok := userResponses[requestBody.Username]; ok {
					w.WriteHeader(tr.status)
					_, err := w.Write(tr.body)
					require.NoError(t, err)
				} else if tr, ok := userResponses[requestBody.Krb5Principal]; ok {
					w.WriteHeader(tr.status)
					_, err := w.Write(tr.body)
					require.NoError(t, err)
				} else if tr, ok := keyResponses[requestBody.KeyId]; ok {
					w.WriteHeader(tr.status)
					_, err := w.Write(tr.body)
					require.NoError(t, err)
				}
			},
		},
	}

	url := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client
}

func setupWithApiInspector(t *testing.T, inspector func(*Request)) *Client {
	t.Helper()
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var requestBody *Request
				err = json.Unmarshal(b, &requestBody)
				require.NoError(t, err)

				inspector(requestBody)
			},
		},
	}

	url := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client
}

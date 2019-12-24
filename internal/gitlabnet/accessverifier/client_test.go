package accessverifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	pb "gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet"
	"gitlab.com/gitlab-org/gitlab-shell/internal/gitlabnet/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/testhelper"
)

var (
	repo   = "group/private"
	action = commandargs.ReceivePack
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
	client, cleanup := setup(t)
	defer cleanup()

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
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := client.Verify(tc.args, action, repo)
			require.NoError(t, err)

			response := buildExpectedResponse(tc.who)
			require.Equal(t, response, result)
		})
	}
}

func TestGetCustomAction(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()

	args := &commandargs.Shell{GitlabUsername: "custom"}
	result, err := client.Verify(args, action, repo)
	require.NoError(t, err)

	response := buildExpectedResponse("user-1")
	response.Payload = CustomPayload{
		Action: "geo_proxy_to_primary",
		Data: CustomPayloadData{
			ApiEndpoints: []string{"geo/proxy_git_push_ssh/info_refs", "geo/proxy_git_push_ssh/push"},
			Username:     "custom",
			PrimaryRepo:  "https://repo/path",
		},
	}
	response.StatusCode = 300

	require.True(t, response.IsCustomAction())
	require.Equal(t, response, result)
}

func TestErrorResponses(t *testing.T) {
	client, cleanup := setup(t)
	defer cleanup()

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
			resp, err := client.Verify(args, action, repo)

			require.EqualError(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

func setup(t *testing.T) (*Client, func()) {
	testDirCleanup, err := testhelper.PrepareTestRootDir()
	require.NoError(t, err)
	defer testDirCleanup()

	body, err := ioutil.ReadFile(path.Join(testhelper.TestRoot, "responses/allowed.json"))
	require.NoError(t, err)

	allowedWithPayloadPath := path.Join(testhelper.TestRoot, "responses/allowed_with_payload.json")
	bodyWithPayload, err := ioutil.ReadFile(allowedWithPayloadPath)
	require.NoError(t, err)

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				b, err := ioutil.ReadAll(r.Body)
				require.NoError(t, err)

				var requestBody *Request
				require.NoError(t, json.Unmarshal(b, &requestBody))

				switch requestBody.Username {
				case "first":
					_, err = w.Write(body)
					require.NoError(t, err)
				case "second":
					errBody := map[string]interface{}{
						"status":  false,
						"message": "missing user",
					}
					require.NoError(t, json.NewEncoder(w).Encode(errBody))
				case "custom":
					w.WriteHeader(http.StatusMultipleChoices)
					_, err = w.Write(bodyWithPayload)
					require.NoError(t, err)
				}

				switch requestBody.KeyId {
				case "1":
					_, err = w.Write(body)
					require.NoError(t, err)
				case "2":
					w.WriteHeader(http.StatusForbidden)
					errBody := &gitlabnet.ErrorResponse{
						Message: "Not allowed!",
					}
					require.NoError(t, json.NewEncoder(w).Encode(errBody))
				case "3":
					w.Write([]byte("{ \"message\": \"broken json!\""))
				case "4":
					w.WriteHeader(http.StatusForbidden)
				}
			},
		},
	}

	url, cleanup := testserver.StartSocketHttpServer(t, requests)

	client, err := NewClient(&config.Config{GitlabUrl: url})
	require.NoError(t, err)

	return client, cleanup
}

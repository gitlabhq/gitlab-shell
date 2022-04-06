package sshd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)

type fakeChannel struct {
	stdErr             io.ReadWriter
	sentRequestName    string
	sentRequestPayload []byte
}

func (f *fakeChannel) Read(data []byte) (int, error) {
	return 0, nil
}

func (f *fakeChannel) Write(data []byte) (int, error) {
	return 0, nil
}

func (f *fakeChannel) Close() error {
	return nil
}

func (f *fakeChannel) CloseWrite() error {
	return nil
}

func (f *fakeChannel) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	f.sentRequestName = name
	f.sentRequestPayload = payload

	return true, nil
}

func (f *fakeChannel) Stderr() io.ReadWriter {
	return f.stdErr
}

var requests = []testserver.TestRequestHandler{
	{
		Path: "/api/v4/internal/discover",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"id": 1000, "name": "Test User", "username": "test-user"}`))
		},
	},
}

func TestHandleEnv(t *testing.T) {
	testCases := []struct {
		desc                    string
		payload                 []byte
		expectedProtocolVersion string
		expectedResult          bool
	}{
		{
			desc:                    "invalid payload",
			payload:                 []byte("invalid"),
			expectedProtocolVersion: "1",
			expectedResult:          false,
		}, {
			desc:                    "valid payload",
			payload:                 ssh.Marshal(envRequest{Name: "GIT_PROTOCOL", Value: "2"}),
			expectedProtocolVersion: "2",
			expectedResult:          true,
		}, {
			desc:                    "valid payload with forbidden env var",
			payload:                 ssh.Marshal(envRequest{Name: "GIT_PROTOCOL_ENV", Value: "2"}),
			expectedProtocolVersion: "1",
			expectedResult:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			s := &session{gitProtocolVersion: "1"}
			r := &ssh.Request{Payload: tc.payload}

			require.Equal(t, s.handleEnv(context.Background(), r), tc.expectedResult)
			require.Equal(t, s.gitProtocolVersion, tc.expectedProtocolVersion)
		})
	}
}

func TestHandleExec(t *testing.T) {
	testCases := []struct {
		desc               string
		payload            []byte
		expectedExecCmd    string
		sentRequestName    string
		sentRequestPayload []byte
	}{
		{
			desc:            "invalid payload",
			payload:         []byte("invalid"),
			expectedExecCmd: "",
			sentRequestName: "",
		}, {
			desc:               "valid payload",
			payload:            ssh.Marshal(execRequest{Command: "discover"}),
			expectedExecCmd:    "discover",
			sentRequestName:    "exit-status",
			sentRequestPayload: ssh.Marshal(exitStatusReq{ExitStatus: 0}),
		},
	}

	url := testserver.StartHttpServer(t, requests)

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			out := &bytes.Buffer{}
			f := &fakeChannel{stdErr: out}
			s := &session{
				gitlabKeyId: "root",
				channel:     f,
				cfg:         &config.Config{GitlabUrl: url},
			}
			r := &ssh.Request{Payload: tc.payload}

			require.Equal(t, false, s.handleExec(context.Background(), r))
			require.Equal(t, tc.sentRequestName, f.sentRequestName)
			require.Equal(t, tc.sentRequestPayload, f.sentRequestPayload)
		})
	}
}

func TestHandleShell(t *testing.T) {
	testCases := []struct {
		desc             string
		cmd              string
		errMsg           string
		gitlabKeyId      string
		expectedExitCode uint32
	}{
		{
			desc:             "fails to parse command",
			cmd:              `\`,
			errMsg:           "Failed to parse command: Invalid SSH command: invalid command line string\nUnknown command: \\\n",
			gitlabKeyId:      "root",
			expectedExitCode: 128,
		}, {
			desc:             "specified command is unknown",
			cmd:              "unknown-command",
			errMsg:           "Unknown command: unknown-command\n",
			gitlabKeyId:      "root",
			expectedExitCode: 128,
		}, {
			desc:             "fails to parse command",
			cmd:              "discover",
			gitlabKeyId:      "",
			errMsg:           "remote: ERROR: Failed to get username: who='' is invalid\n",
			expectedExitCode: 1,
		}, {
			desc:             "fails to parse command",
			cmd:              "discover",
			errMsg:           "",
			gitlabKeyId:      "root",
			expectedExitCode: 0,
		},
	}

	url := testserver.StartHttpServer(t, requests)

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			out := &bytes.Buffer{}
			s := &session{
				gitlabKeyId: tc.gitlabKeyId,
				execCmd:     tc.cmd,
				channel:     &fakeChannel{stdErr: out},
				cfg:         &config.Config{GitlabUrl: url},
			}
			r := &ssh.Request{}

			require.Equal(t, tc.expectedExitCode, s.handleShell(context.Background(), r))
			require.Equal(t, tc.errMsg, out.String())
		})
	}
}

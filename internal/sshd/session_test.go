package sshd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/console"
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
		expectedErr             error
		expectedProtocolVersion string
		expectedResult          bool
	}{
		{
			desc:                    "invalid payload",
			payload:                 []byte("invalid"),
			expectedErr:             errors.New("ssh: unmarshal error for field Name of type envRequest"),
			expectedProtocolVersion: "1",
			expectedResult:          false,
		}, {
			desc:                    "valid payload",
			payload:                 ssh.Marshal(envRequest{Name: "GIT_PROTOCOL", Value: "2"}),
			expectedErr:             nil,
			expectedProtocolVersion: "2",
			expectedResult:          true,
		}, {
			desc:                    "valid payload with forbidden env var",
			payload:                 ssh.Marshal(envRequest{Name: "GIT_PROTOCOL_ENV", Value: "2"}),
			expectedErr:             nil,
			expectedProtocolVersion: "1",
			expectedResult:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			s := &session{gitProtocolVersion: "1"}
			r := &ssh.Request{Payload: tc.payload}

			shouldContinue, err := s.handleEnv(context.Background(), r)

			require.Equal(t, tc.expectedErr, err)
			require.Equal(t, tc.expectedResult, shouldContinue)
			require.Equal(t, tc.expectedProtocolVersion, s.gitProtocolVersion)
		})
	}
}

func TestHandleExec(t *testing.T) {
	testCases := []struct {
		desc               string
		payload            []byte
		expectedErr        error
		expectedExecCmd    string
		sentRequestName    string
		sentRequestPayload []byte
	}{
		{
			desc:            "invalid payload",
			payload:         []byte("invalid"),
			expectedErr:     errors.New("ssh: unmarshal error for field Command of type execRequest"),
			expectedExecCmd: "",
			sentRequestName: "",
		}, {
			desc:               "valid payload",
			payload:            ssh.Marshal(execRequest{Command: "discover"}),
			expectedErr:        nil,
			expectedExecCmd:    "discover",
			sentRequestName:    "exit-status",
			sentRequestPayload: ssh.Marshal(exitStatusReq{ExitStatus: 0}),
		},
	}

	url := testserver.StartHttpServer(t, requests)

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			sessions := []*session{
				{
					gitlabKeyId: "root",
					cfg:         &config.Config{GitlabUrl: url},
				},
				{
					gitlabKrb5Principal: "test@TEST.TEST",
					cfg:                 &config.Config{GitlabUrl: url},
				},
			}
			for _, s := range sessions {
				out := &bytes.Buffer{}
				f := &fakeChannel{stdErr: out}
				r := &ssh.Request{Payload: tc.payload}

				s.channel = f
				shouldContinue, err := s.handleExec(context.Background(), r)

				require.Equal(t, tc.expectedErr, err)
				require.Equal(t, false, shouldContinue)
				require.Equal(t, tc.sentRequestName, f.sentRequestName)
				require.Equal(t, tc.sentRequestPayload, f.sentRequestPayload)
			}
		})
	}
}

func TestHandleShell(t *testing.T) {
	testCases := []struct {
		desc              string
		cmd               string
		errMsg            string
		gitlabKeyId       string
		expectedErrString string
		expectedExitCode  uint32
	}{
		{
			desc:              "fails to parse command",
			cmd:               `\`,
			errMsg:            "ERROR: Failed to parse command: Invalid SSH command: invalid command line string\n",
			gitlabKeyId:       "root",
			expectedErrString: "Invalid SSH command: invalid command line string",
			expectedExitCode:  128,
		}, {
			desc:              "specified command is unknown",
			cmd:               "unknown-command",
			errMsg:            "ERROR: Unknown command: unknown-command\n",
			gitlabKeyId:       "root",
			expectedErrString: "Disallowed command",
			expectedExitCode:  128,
		}, {
			desc:              "fails to parse command",
			cmd:               "discover",
			gitlabKeyId:       "",
			errMsg:            "ERROR: Failed to get username: who='' is invalid\n",
			expectedErrString: "Failed to get username: who='' is invalid",
			expectedExitCode:  1,
		}, {
			desc:              "fails to parse command",
			cmd:               "discover",
			errMsg:            "",
			gitlabKeyId:       "root",
			expectedErrString: "",
			expectedExitCode:  0,
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

			exitCode, err := s.handleShell(context.Background(), r)

			if tc.expectedErrString != "" {
				require.Equal(t, tc.expectedErrString, err.Error())
			}

			require.Equal(t, tc.expectedExitCode, exitCode)

			formattedErr := &bytes.Buffer{}
			if tc.errMsg != "" {
				console.DisplayWarningMessage(tc.errMsg, formattedErr)
				require.Equal(t, formattedErr.String(), out.String())
			} else {
				require.Equal(t, tc.errMsg, out.String())
			}
		})
	}
}

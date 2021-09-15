package sshd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

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

			require.Equal(t, s.handleEnv(r), tc.expectedResult)
			require.Equal(t, s.gitProtocolVersion, tc.expectedProtocolVersion)
		})
	}
}

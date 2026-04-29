package gitlabnet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGlID(t *testing.T) {
	tests := []struct {
		desc              string
		input             string
		wantUserID        int
		wantIsUser        bool
		wantDeployTokenID int
		wantIsDeployToken bool
		wantErr           bool
	}{
		{desc: "user-1", input: "user-1", wantUserID: 1, wantIsUser: true},
		{desc: "user-42", input: "user-42", wantUserID: 42, wantIsUser: true},
		{desc: "deploy-token-52", input: "deploy-token-52", wantDeployTokenID: 52, wantIsDeployToken: true},
		{desc: "deploy-token-1", input: "deploy-token-1", wantDeployTokenID: 1, wantIsDeployToken: true},
		{desc: "user- without number", input: "user-", wantErr: true},
		{desc: "user-abc", input: "user-abc", wantErr: true},
		{desc: "deploy-token- without number", input: "deploy-token-", wantErr: true},
		{desc: "deploy-token-abc", input: "deploy-token-abc", wantErr: true},
		{desc: "key-1 unknown prefix", input: "key-1", wantErr: true},
		{desc: "empty string", input: "", wantErr: true},
		{desc: "plain numeric", input: "1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got, err := ParseGlID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)

			userID, isUser := got.UserID()
			require.Equal(t, tt.wantUserID, userID)
			require.Equal(t, tt.wantIsUser, isUser)

			deployTokenID, isDeployToken := got.DeployTokenID()
			require.Equal(t, tt.wantDeployTokenID, deployTokenID)
			require.Equal(t, tt.wantIsDeployToken, isDeployToken)
		})
	}
}

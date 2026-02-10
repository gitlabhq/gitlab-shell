package gitlab

import (
	"context"
	"net/http"
	"testing"

	"github.com/elliotforbes/fakes"
	"github.com/stretchr/testify/require"
)

func TestGetUser(t *testing.T) {
	testCases := []struct {
		name          string
		endpoint      *fakes.Endpoint
		expectedUser  *User
		userArgs      GetUserArgs
		expectedError error
	}{
		{
			name:     "can successfully retrieve user with username",
			userArgs: GetUserArgs{GitlabUsername: "@alex-doe"},
			endpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/discover",
				Response: `{"id": 5, "name": "alex doe", "username": "@alex-doe"}`,
				Expectation: func(r *http.Request) {
					// ensure that the username query param has been set correctly
					require.Equal(t, "@alex-doe", r.URL.Query().Get("username"))
				},
			},
			expectedUser: &User{UserID: 5, Name: "alex doe", Username: "@alex-doe"},
		},
		{
			name:     "can successfully retrieve user with key_id",
			userArgs: GetUserArgs{GitlabKeyID: "5"},
			endpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/discover",
				Response: `{"id": 5, "name": "alex doe", "username": "@alex-doe"}`,
				Expectation: func(r *http.Request) {
					require.Equal(t, "5", r.URL.Query().Get("key_id"))
				},
			},
			expectedUser: &User{UserID: 5, Name: "alex doe", Username: "@alex-doe"},
		},
		{
			name:     "can successfully retrieve user with krb5principal set",
			userArgs: GetUserArgs{GitlabKrb5Principal: "john-doe@TEST.TEST"},
			endpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/discover",
				Response: `{"id": 5, "name": "alex doe", "username": "@alex-doe"}`,
				Expectation: func(r *http.Request) {
					require.Equal(t, "john-doe@TEST.TEST", r.URL.Query().Get("krb5principal"))
				},
			},
			expectedUser: &User{UserID: 5, Name: "alex doe", Username: "@alex-doe"},
		},
		{
			name:          "handles error cases gracefully",
			userArgs:      GetUserArgs{},
			expectedError: ErrInvalidWho,
		},
	}

	for _, tc := range testCases {
		gitlabAPI := fakes.New()
		if tc.endpoint != nil {
			gitlabAPI.Endpoint(tc.endpoint)
		}
		gitlabAPI.Run(t)

		client, err := New(ClientOpts{
			GitlabURL: gitlabAPI.BaseURL,
		})
		require.NoError(t, err)

		t.Run(tc.name, func(t *testing.T) {
			user, err := client.GetUser(context.Background(), tc.userArgs)
			if tc.expectedError != nil {
				require.ErrorIs(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedUser, user)
			}
		})
		// ensures we clean up the in-memory fake services between runs
		gitlabAPI.TidyUp(t)
	}
}

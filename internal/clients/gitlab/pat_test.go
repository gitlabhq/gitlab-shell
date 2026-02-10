package gitlab

import (
	"context"
	"net/http"
	"testing"

	"github.com/elliotforbes/fakes"
	"github.com/stretchr/testify/require"
)

func TestGetPAT(t *testing.T) {
	testCases := []struct {
		name          string
		userEndpoint  *fakes.Endpoint
		patEndpoint   *fakes.Endpoint
		expectedPAT   *PersonalAccessTokenResponse
		patArgs       GetPATArgs
		expectedError string
	}{
		{
			name:    "can successfully retrieve the user and then the corresponding PAT",
			patArgs: GetPATArgs{GitlabUsername: "@alex-doe"},
			// this test first hits the discover API endpoint to retrieve the user
			userEndpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/discover",
				Response: `{"id": 5, "name": "alex doe", "username": "@alex-doe"}`,
				Expectation: func(r *http.Request) {
					require.Equal(t, "@alex-doe", r.URL.Query().Get("username"))
				},
			},
			// then onto the PAT endpoint to fetch the personal access token
			patEndpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/personal_access_token",
				Response: `{"success": true, "token": "aAY1G3YPeemECgUvxuXY", "expires_at": "9001-11-17", "scopes": ["read_api", "read_repository"]}`,
			},
			expectedPAT: &PersonalAccessTokenResponse{
				Success:   true,
				Token:     "aAY1G3YPeemECgUvxuXY",
				Scopes:    []string{"read_api", "read_repository"},
				ExpiresAt: "9001-11-17",
			},
		},
		{
			name:    "user cannot be retrieved",
			patArgs: GetPATArgs{GitlabUsername: "@alex-doe"},
			// this test first hits the discover API endpoint to retrieve the user
			userEndpoint: &fakes.Endpoint{
				Path:       "/api/v4/internal/discover",
				Response:   `{"message": "user not found"}`,
				StatusCode: http.StatusNotFound,
				Expectation: func(r *http.Request) {
					require.Equal(t, "@alex-doe", r.URL.Query().Get("username"))
				},
			},
			// the PAT endpoint is then never called due to a failure to retrieve
			// the user.
			expectedError: "user not found",
			patEndpoint:   nil,
			expectedPAT:   nil,
		},
		{
			name:    "can successfully retrieve the PAT by key_id",
			patArgs: GetPATArgs{GitlabKeyID: "5"},
			// if we supply the key_id, then the expectation is that we do not
			// need to retrieve the user and we skip this endpoint call
			userEndpoint: nil,
			// then onto the PAT endpoint to fetch the personal access token
			patEndpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/personal_access_token",
				Response: `{"success": true, "token": "aAY1G3YPeemECgUvxuXY", "expires_at": "9001-11-17", "scopes": ["read_api", "read_repository"]}`,
			},
			expectedPAT: &PersonalAccessTokenResponse{
				Success:   true,
				Token:     "aAY1G3YPeemECgUvxuXY",
				Scopes:    []string{"read_api", "read_repository"},
				ExpiresAt: "9001-11-17",
			},
		},
		{
			name:    "will return not allowed if the user is not allowed to create a PAT",
			patArgs: GetPATArgs{GitlabUsername: "@alex-doe"},
			// this test first hits the discover API endpoint to retrieve the user
			userEndpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/discover",
				Response: `{"id": 5, "name": "alex doe", "username": "@alex-doe"}`,
				Expectation: func(r *http.Request) {
					require.Equal(t, "@alex-doe", r.URL.Query().Get("username"))
				},
			},
			// then onto the PAT endpoint to fetch the personal access token
			patEndpoint: &fakes.Endpoint{
				Path:       "/api/v4/internal/personal_access_token",
				StatusCode: http.StatusForbidden,
				Response:   `{"message": "Not allowed!"}`,
			},
			expectedPAT:   nil,
			expectedError: "Not allowed!",
		},
		{
			name:    "will return parsing failed if the response is not valid JSON",
			patArgs: GetPATArgs{GitlabUsername: "@alex-doe"},
			// this test first hits the discover API endpoint to retrieve the user
			userEndpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/discover",
				Response: `{"id": 5, "name": "alex doe", "username": "@alex-doe"}`,
				Expectation: func(r *http.Request) {
					require.Equal(t, "@alex-doe", r.URL.Query().Get("username"))
				},
			},
			// then onto the PAT endpoint to fetch the personal access token
			patEndpoint: &fakes.Endpoint{
				Path:     "/api/v4/internal/personal_access_token",
				Response: `{"broken_json}`,
			},
			expectedPAT:   nil,
			expectedError: "parsing failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gitlabAPI := fakes.New()
			if tc.userEndpoint != nil {
				gitlabAPI.Endpoint(tc.userEndpoint)
			}
			if tc.patEndpoint != nil {
				gitlabAPI.Endpoint(tc.patEndpoint)
			}
			gitlabAPI.Run(t)

			client, err := New(ClientOpts{
				GitlabURL: gitlabAPI.BaseURL,
			})
			require.NoError(t, err)
			pat, err := client.GetPersonalAccessToken(context.Background(), tc.patArgs)
			if tc.expectedError != "" {
				require.EqualError(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedPAT, pat)
			}
			// ensures we clean up the in-memory fake services between runs
			gitlabAPI.TidyUp(t)
		})
	}
}

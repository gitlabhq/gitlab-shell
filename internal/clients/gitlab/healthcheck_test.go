package gitlab_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
)

func TestHealthcheckClient_Check_Success(t *testing.T) {
	expectedResponse := gitlab.HealthcheckResponse{
		APIVersion:     "1.0",
		GitlabVersion:  "15.0.0",
		GitlabRevision: "abc123",
		Redis:          true,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(expectedResponse); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer srv.Close()

	hc := gitlab.NewHealthcheckClient(newTestClient(t, srv))
	resp, err := hc.Check(context.Background())

	require.NoError(t, err)
	require.Equal(t, expectedResponse.Redis, resp.Redis)
	require.Equal(t, expectedResponse.APIVersion, resp.APIVersion)
}

func TestHealthcheckClient_Check_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	hc := gitlab.NewHealthcheckClient(newTestClient(t, srv))
	_, err := hc.Check(context.Background())

	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "Internal API error (502)", apiErr.Msg)
}

func TestHealthcheckClient_Check_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer srv.Close()

	hc := gitlab.NewHealthcheckClient(newTestClient(t, srv))
	_, err := hc.Check(context.Background())
	require.EqualError(t, err, "parsing failed")
}

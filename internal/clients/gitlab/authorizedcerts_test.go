package gitlab_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/clients/gitlab"
)

func TestAuthorizedCertsClient_GetByKey_Success(t *testing.T) {
	var capturedPath, capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"alice","namespace":"alice-group"}`))
	}))
	defer srv.Close()

	c := gitlab.NewAuthorizedCertsClient(newTestClient(t, srv))
	resp, err := c.GetByKey(context.Background(), "user-1", "fp-abc")

	require.NoError(t, err)
	require.Equal(t, "alice", resp.Username)
	require.Equal(t, "alice-group", resp.Namespace)
	require.Equal(t, "/api/v4/internal/authorized_certs", capturedPath)
	require.Equal(t, "key=fp-abc&user_identifier=user-1", capturedQuery)
}

func TestAuthorizedCertsClient_GetByKey_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := gitlab.NewAuthorizedCertsClient(newTestClient(t, srv))
	_, err := c.GetByKey(context.Background(), "user-1", "fp-abc")

	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "Internal API error (403)", apiErr.Msg)
}

func TestAuthorizedCertsClient_GetByKey_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer srv.Close()

	c := gitlab.NewAuthorizedCertsClient(newTestClient(t, srv))
	_, err := c.GetByKey(context.Background(), "user-1", "fp-abc")
	require.EqualError(t, err, "parsing failed")
}

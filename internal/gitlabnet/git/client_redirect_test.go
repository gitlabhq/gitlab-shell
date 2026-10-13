package git

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
)

// TestPackRequestsDoNotFollowRedirects guards against replaying a pack body,
// and the Shell JWT header copied with it, to a redirect target. A replayable
// body sets GetBody, which would otherwise let http.Client follow 307/308.
func TestPackRequestsDoNotFollowRedirects(t *testing.T) {
	for _, code := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var targetHits atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				targetHits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(target.Close)

			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				http.Redirect(w, r, target.URL+r.URL.Path, code)
			}))
			t.Cleanup(origin.Close)

			body, reader := NewReplayBody(DefaultReplayLimit)
			_, err := io.WriteString(body, requestBody)
			require.NoError(t, err)
			require.NoError(t, body.Close())

			gitClient := &Client{URL: origin.URL, Headers: map[string]string{"Gitlab-Shell-Api-Request": "jwt"}}
			response, err := gitClient.SSHUploadPack(context.Background(), reader) //nolint:bodyclose // no response on error
			require.Nil(t, response)

			var apiErr *client.APIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, code, apiErr.StatusCode)
			require.True(t, apiErr.System, "a refused redirect is a system error")
			require.EqualValues(t, 0, targetHits.Load(), "the redirect target must never be reached")
		})
	}
}

func TestInfoRefsStillFollowsRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("refs for " + r.URL.Query().Get("service")))
	}))
	t.Cleanup(target.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)

	response, err := (&Client{URL: origin.URL}).InfoRefs(context.Background(), "git-upload-pack")
	require.NoError(t, err)
	defer response.Body.Close()

	result, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(result), "refs for git-upload-pack"))
}

func TestCheckRedirectLimit(t *testing.T) {
	via := make([]*http.Request, maxGetRedirects)
	for i := range via {
		via[i] = &http.Request{Method: http.MethodGet}
	}
	require.NoError(t, checkRedirect(&http.Request{}, via[:maxGetRedirects-1]))
	require.EqualError(t, checkRedirect(&http.Request{}, via), "stopped after 10 redirects")
}

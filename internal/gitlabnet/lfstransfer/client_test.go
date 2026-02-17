package lfstransfer

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

func TestBatch_FollowsRedirectPreservingPOSTMethod(t *testing.T) {
	testCases := []struct {
		name         string
		redirectCode int
	}{
		{"301 Moved Permanently", http.StatusMovedPermanently},
		{"302 Found", http.StatusFound},
		{"307 Temporary Redirect", http.StatusTemporaryRedirect},
		{"308 Permanent Redirect", http.StatusPermanentRedirect},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var receivedMethod string
			var receivedBody []byte
			var receivedContentType string

			// Final server that receives the redirected request
			finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				receivedBody, _ = io.ReadAll(r.Body)
				receivedContentType = r.Header.Get("Content-Type")

				response := &BatchResponse{
					Objects: []*BatchObject{{Oid: "test-oid", Size: 100}},
				}
				w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
				assert.NoError(t, json.NewEncoder(w).Encode(response))
			}))
			defer finalServer.Close()

			// Redirect server
			redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, finalServer.URL+"/objects/batch", tc.redirectCode)
			}))
			defer redirectServer.Close()

			client, err := NewClient(
				&config.Config{},
				&commandargs.Shell{},
				redirectServer.URL,
				"Basic test-auth",
			)
			require.NoError(t, err)

			objects := []*BatchObject{{Oid: "test-oid", Size: 100}}
			_, err = client.Batch("download", objects, "refs/heads/main", "sha256")

			require.NoError(t, err)
			assert.Equal(t, http.MethodPost, receivedMethod, "POST method should be preserved after redirect")
			assert.Equal(t, ClientHeader, receivedContentType, "Content-Type header should be preserved")
			assert.NotEmpty(t, receivedBody, "Request body should be preserved after redirect")

			// Verify the body content is correct
			var receivedRequest batchRequest
			err = json.Unmarshal(receivedBody, &receivedRequest)
			require.NoError(t, err)
			assert.Equal(t, "download", receivedRequest.Operation)
			assert.Len(t, receivedRequest.Objects, 1)
			assert.Equal(t, "test-oid", receivedRequest.Objects[0].Oid)
		})
	}
}

func TestBatch_PreservesAuthorizationHeaderOnSameHostRedirect(t *testing.T) {
	var receivedAuth string

	// Use a single server with a mux to handle both redirect and final request (same host)
	mux := http.NewServeMux()
	mux.HandleFunc("/start/objects/batch", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final/objects/batch", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/final/objects/batch", func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		response := &BatchResponse{Objects: []*BatchObject{}}
		w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
		_ = json.NewEncoder(w).Encode(response)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(
		&config.Config{},
		&commandargs.Shell{},
		server.URL+"/start",
		"Basic test-auth-token",
	)
	require.NoError(t, err)

	_, err = client.Batch("download", []*BatchObject{}, "", "sha256")
	require.NoError(t, err)
	assert.Equal(t, "Basic test-auth-token", receivedAuth, "Authorization header should be preserved on same-host redirect")
}

func TestBatch_EnforcesMaxRedirectLimit(t *testing.T) {
	redirectCount := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		http.Redirect(w, r, server.URL+"/objects/batch", http.StatusFound)
	}))
	defer server.Close()

	client, err := NewClient(
		&config.Config{},
		&commandargs.Shell{},
		server.URL,
		"Basic test-auth",
	)
	require.NoError(t, err)

	_, err = client.Batch("download", []*BatchObject{}, "", "sha256")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect")
	assert.LessOrEqual(t, redirectCount, 11, "Should stop after 10 redirects")
}

func TestBatch_DoesNotLeakAuthorizationHeaderOnCrossHostRedirect(t *testing.T) {
	var receivedAuth string

	// Final server on a different "host" (different port simulates different host)
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		response := &BatchResponse{Objects: []*BatchObject{}}
		w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer finalServer.Close()

	// Redirect server
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to different host (finalServer has different port)
		http.Redirect(w, r, finalServer.URL+"/objects/batch", http.StatusTemporaryRedirect)
	}))
	defer redirectServer.Close()

	client, err := NewClient(
		&config.Config{},
		&commandargs.Shell{},
		redirectServer.URL,
		"Basic secret-auth-token",
	)
	require.NoError(t, err)

	_, err = client.Batch("download", []*BatchObject{}, "", "sha256")
	require.NoError(t, err)

	// Authorization header should NOT be sent to different host
	assert.Empty(t, receivedAuth, "Authorization header should not be leaked to different host")
}

func TestLock_FollowsRedirectPreservingPOSTMethod(t *testing.T) {
	var receivedMethod string
	var receivedBody []byte

	// Final server
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedBody, _ = io.ReadAll(r.Body)

		response := map[string]interface{}{
			"lock": map[string]interface{}{
				"id":        "lock1",
				"path":      "/test/file",
				"locked_at": "2024-01-01T00:00:00Z",
				"owner":     map[string]string{"name": "testuser"},
			},
		}
		w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer finalServer.Close()

	// Redirect server
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL+"/locks", http.StatusFound)
	}))
	defer redirectServer.Close()

	client, err := NewClient(
		&config.Config{},
		&commandargs.Shell{},
		redirectServer.URL,
		"Basic test-auth",
	)
	require.NoError(t, err)

	_, err = client.Lock("/test/file", "refs/heads/main")

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, receivedMethod, "POST method should be preserved after redirect")
	assert.NotEmpty(t, receivedBody, "Request body should be preserved after redirect")
}

func TestPutObject_FollowsRedirectPreservingPUTMethod(t *testing.T) {
	var receivedMethod string
	var receivedBody []byte

	// Final server
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer finalServer.Close()

	// Redirect server
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL+"/object", http.StatusTemporaryRedirect)
	}))
	defer redirectServer.Close()

	client, err := NewClient(
		&config.Config{},
		&commandargs.Shell{},
		redirectServer.URL,
		"Basic test-auth",
	)
	require.NoError(t, err)

	testData := []byte("test file content")
	err = client.PutObject("test-oid", redirectServer.URL+"/object", map[string]string{
		"Authorization": "Basic 1234567890",
	}, io.NopCloser(bytes.NewReader(testData)))

	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, receivedMethod, "PUT method should be preserved after redirect")
	assert.Equal(t, testData, receivedBody, "Request body should be preserved after redirect")
}

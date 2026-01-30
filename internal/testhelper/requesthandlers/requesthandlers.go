// Package requesthandlers provides functions for building test request handlers.
package requesthandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client/testserver"
)

// BuildDisallowedByAPIHandlers returns test request handlers for disallowed API calls.
func BuildDisallowedByAPIHandlers(t *testing.T) []testserver.TestRequestHandler {
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				body := map[string]interface{}{
					"status":  false,
					"message": "Disallowed by API call",
				}
				w.WriteHeader(http.StatusForbidden)
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
	}

	return requests
}

// BuildAllowedWithGitalyHandlers returns test request handlers for allowed API calls with Gitaly.
func BuildAllowedWithGitalyHandlers(t *testing.T, gitalyAddress string) []testserver.TestRequestHandler {
	return BuildAllowedWithGitalyHandlersAndRetryConfig(t, gitalyAddress, nil)
}

// BuildAllowedWithGitalyHandlersAndRetryConfig returns test request handlers for allowed API calls with Gitaly and retry config.
func BuildAllowedWithGitalyHandlersAndRetryConfig(t *testing.T, gitalyAddress string, retryConfig map[string]interface{}) []testserver.TestRequestHandler {
	body := map[string]interface{}{
		"status":      true,
		"gl_id":       "1",
		"gl_key_type": "key",
		"gl_key_id":   123,
		"gl_username": "alex-doe",
		"gitaly": map[string]interface{}{
			"repository": map[string]interface{}{
				"storage_name":                     "storage_name",
				"relative_path":                    "relative_path",
				"git_object_directory":             "path/to/git_object_directory",
				"git_alternate_object_directories": []string{"path/to/git_alternate_object_directory"},
				"gl_repository":                    "group/repo",
				"gl_project_path":                  "group/project-path",
			},
			"address": gitalyAddress,
			"token":   "token",
			"features": map[string]string{
				"gitaly-feature-cache_invalidator":        "true",
				"gitaly-feature-inforef_uploadpack_cache": "false",
			},
		},
	}

	if retryConfig != nil {
		body["retry_config"] = retryConfig
	}

	return []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
	}
}

// BuildAllowedWithCustomActionsHandlers returns test request handlers for allowed API calls with custom actions.
func BuildAllowedWithCustomActionsHandlers(t *testing.T) []testserver.TestRequestHandler {
	// Create a separate HTTP server for Git protocol responses
	gitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info/refs" && r.URL.Query().Get("service") == "git-receive-pack" {
			// Proper Git info/refs response format
			w.Header().Set("Content-Type", "application/x-git-receive-pack-advertisement")
			_, err := w.Write([]byte("001f# service=git-receive-pack\n0000\n0045abcdef1234567890abcdef1234567890abcdef1234 refs/heads/master\n0000"))
			assert.NoError(t, err)
		} else if r.URL.Path == "/git-receive-pack" {
			// Proper Git receive-pack response format
			w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
			_, err := w.Write([]byte("0008NAK\n0019ok refs/heads/master\n0000"))
			assert.NoError(t, err)
		}
	}))
	t.Cleanup(func() { gitServer.Close() })

	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				body := map[string]interface{}{
					"status": true,
					"gl_id":  "1",
					"payload": map[string]interface{}{
						"action": "geo_proxy_to_primary",
						"data": map[string]interface{}{
							"primary_repo": gitServer.URL,
							"gl_username":  "custom",
						},
					},
				}
				w.WriteHeader(http.StatusMultipleChoices)
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
	}

	return requests
}

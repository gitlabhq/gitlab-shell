// Package requesthandlers provides functions for building test request handlers.
package requesthandlers

import (
	"encoding/json"
	"net/http"
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
	requests := []testserver.TestRequestHandler{
		{
			Path: "/api/v4/internal/allowed",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
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
							"some-other-ff":                           "true",
						},
					},
				}
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
	}

	return requests
}

// BuildAllowedWithCustomActionsHandlers returns test request handlers for allowed API calls with custom actions.
func BuildAllowedWithCustomActionsHandlers(t *testing.T) []testserver.TestRequestHandler {
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
							"api_endpoints": []string{"/geo/proxy/info_refs", "/geo/proxy/push"},
							"gl_username":   "custom",
							"primary_repo":  "https://repo/path",
						},
					},
				}
				w.WriteHeader(http.StatusMultipleChoices)
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
		{
			Path: "/geo/proxy/info_refs",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				body := map[string]interface{}{"result": []byte("custom")}
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
		{
			Path: "/geo/proxy/push",
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				body := map[string]interface{}{"result": []byte("output")}
				assert.NoError(t, json.NewEncoder(w).Encode(body))
			},
		},
	}

	return requests
}

package githttp

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type capturedRequest struct {
	path    string
	headers http.Header
	body    []byte
}

func startCapturingServer(t *testing.T, responseBody string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.headers = r.Header.Clone()
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		captured.body = body
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(responseBody))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func TestWithGitProtocolHeader(t *testing.T) {
	testCases := []struct {
		desc    string
		headers map[string]string
		version string
		want    map[string]string
	}{
		{
			desc:    "nil headers",
			version: testGitProtocolVersion,
			want:    map[string]string{gitProtocolHeader: testGitProtocolVersion},
		},
		{
			desc:    "existing headers are preserved and not mutated",
			headers: map[string]string{"Existing-Header": "value"},
			version: testGitProtocolVersion,
			want:    map[string]string{"Existing-Header": "value", gitProtocolHeader: testGitProtocolVersion},
		},
		{
			desc:    "empty version is set",
			headers: map[string]string{gitProtocolHeader: testGitProtocolVersion},
			want:    map[string]string{gitProtocolHeader: ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			originalHeaders := maps.Clone(tc.headers)

			result := withGitProtocolHeader(tc.headers, tc.version)

			assert.Equal(t, tc.want, result)
			assert.Equal(t, originalHeaders, tc.headers)
		})
	}
}

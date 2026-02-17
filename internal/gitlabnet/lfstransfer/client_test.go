package lfstransfer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
)

const responseCancellationTimeout = 5 * time.Second

// retryablehttp retries other 5xx responses, which would rerun this single-use handler and hang or panic.
var nonRetryableStatusCodes = []int{http.StatusBadRequest, http.StatusNotImplemented}

func newStreamingServer(t *testing.T, statusCode int) (*httptest.Server, <-chan struct{}) {
	t.Helper()

	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()

		<-r.Context().Done()
		close(requestCanceled)
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})

	return server, requestCanceled
}

func requireSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(responseCancellationTimeout):
		require.Fail(t, "timed out waiting for signal")
	}
}

func TestNewClient(t *testing.T) {
	client, err := NewClient(nil, nil, "https://example.com", "authorization")
	require.NoError(t, err)
	require.NotNil(t, client.client)
	require.Equal(t, 3, client.client.RetryMax)
	require.Nil(t, client.client.Logger)
}

func TestNewAuthenticatedPostRequest(t *testing.T) {
	client, err := NewClient(nil, nil, "", "custom-authorization")
	require.NoError(t, err)

	for _, tc := range []struct {
		desc    string
		url     string
		wantErr bool
	}{
		{
			desc: "authenticated request",
			url:  "https://example.com/locks",
		},
		{
			desc:    "invalid url",
			url:     "://example.com/locks",
			wantErr: true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			req, err := client.newAuthenticatedPostRequest(tc.url, strings.NewReader("body"))

			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, req)
				return
			}

			require.NoError(t, err)
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "https://example.com/locks", req.URL.String())
			require.Equal(t, ClientHeader, req.Header.Get("Content-Type"))
			require.Len(t, req.Header.Values("Content-Type"), 1)
			require.Equal(t, "custom-authorization", req.Header.Get("Authorization"))
			require.Len(t, req.Header.Values("Authorization"), 1)
		})
	}
}

func TestBatchClosesErrorResponseBody(t *testing.T) {
	requireClosesErrorResponseBody(t, func(t *testing.T, client *Client, statusCode int) {
		response, err := client.Batch("download", nil, "", "")

		require.ErrorContains(t, err, fmt.Sprintf("response failed with status code: %d", statusCode))
		require.Nil(t, response)
	})
}

func TestGetObjectClosesErrorResponseBody(t *testing.T) {
	requireClosesErrorResponseBody(t, func(t *testing.T, client *Client, _ int) {
		body, size, err := client.GetObject("", client.href, nil)

		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, body)
		require.Zero(t, size)
	})
}

func requireClosesErrorResponseBody(t *testing.T, call func(*testing.T, *Client, int)) {
	t.Helper()

	for _, statusCode := range nonRetryableStatusCodes {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			server, requestCanceled := newStreamingServer(t, statusCode)
			client, err := NewClient(nil, nil, server.URL, "authorization")
			require.NoError(t, err)

			call(t, client, statusCode)
			requireSignal(t, requestCanceled)
		})
	}
}

type receivedRequest struct {
	method      string
	body        []byte
	contentType string
	auth        string
}

var batchRedirectCases = []struct {
	name         string
	redirectCode int
	wantMethod   string
	wantBody     bool
}{
	{"301 Moved Permanently", http.StatusMovedPermanently, http.MethodPost, true},
	{"302 Found", http.StatusFound, http.MethodPost, true},
	{"303 See Other", http.StatusSeeOther, http.MethodGet, false},
	{"307 Temporary Redirect", http.StatusTemporaryRedirect, http.MethodPost, true},
	{"308 Permanent Redirect", http.StatusPermanentRedirect, http.MethodPost, true},
}

func TestBatch_FollowsRedirectsWithStandardSemantics(t *testing.T) {
	for _, tc := range batchRedirectCases {
		t.Run(tc.name, func(t *testing.T) {
			var received receivedRequest
			mux := http.NewServeMux()
			mux.HandleFunc("/start/objects/batch", redirectHandler("/final/objects/batch", tc.redirectCode))
			mux.Handle("/final/objects/batch", batchResponseHandler(t, &received))
			server := httptest.NewServer(mux)
			defer server.Close()

			runBatch(t, server.URL+"/start")

			assert.Equal(t, tc.wantMethod, received.method)
			assert.Equal(t, "Basic test-auth", received.auth)
			if tc.wantBody {
				assert.Equal(t, ClientHeader, received.contentType)
			}
			assertBatchBody(t, received.body, tc.wantBody)
		})
	}
}

func TestBatch_FollowsCrossHostRedirectsWithoutAuthorization(t *testing.T) {
	for _, tc := range batchRedirectCases {
		t.Run(tc.name, func(t *testing.T) {
			var received receivedRequest
			finalServer := httptest.NewServer(batchResponseHandler(t, &received))
			defer finalServer.Close()
			redirectServer := httptest.NewServer(redirectHandler(finalServer.URL+"/objects/batch", tc.redirectCode))
			defer redirectServer.Close()

			runBatch(t, redirectServer.URL)

			assert.Equal(t, tc.wantMethod, received.method)
			assert.Empty(t, received.auth)
			if tc.wantBody {
				assert.Equal(t, ClientHeader, received.contentType)
			}
			assertBatchBody(t, received.body, tc.wantBody)
		})
	}
}

func TestBatch_FollowsRedirectChains(t *testing.T) {
	testCases := []struct {
		name       string
		redirects  []int
		wantMethod string
		wantBody   bool
	}{
		{"301 then 302", []int{http.StatusMovedPermanently, http.StatusFound}, http.MethodPost, true},
		{"301 then 307", []int{http.StatusMovedPermanently, http.StatusTemporaryRedirect}, http.MethodPost, true},
		{"301 then 308", []int{http.StatusMovedPermanently, http.StatusPermanentRedirect}, http.MethodPost, true},
		{"302 then 307", []int{http.StatusFound, http.StatusTemporaryRedirect}, http.MethodPost, true},
		{"302 then 308", []int{http.StatusFound, http.StatusPermanentRedirect}, http.MethodPost, true},
		{"301 then 307 then 308", []int{http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusPermanentRedirect}, http.MethodPost, true},
		{"303 then 302", []int{http.StatusSeeOther, http.StatusFound}, http.MethodGet, false},
		{"302 then 303", []int{http.StatusFound, http.StatusSeeOther}, http.MethodGet, false},
		{"303 then 302 then 307", []int{http.StatusSeeOther, http.StatusFound, http.StatusTemporaryRedirect}, http.MethodGet, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var received receivedRequest
			mux := http.NewServeMux()
			for index, statusCode := range tc.redirects {
				path := fmt.Sprintf("/redirect/%d/objects/batch", index)
				if index == 0 {
					path = "/start/objects/batch"
				}
				target := fmt.Sprintf("/redirect/%d/objects/batch", index+1)
				if index == len(tc.redirects)-1 {
					target = "/final/objects/batch"
				}
				mux.HandleFunc(path, redirectHandler(target, statusCode))
			}
			mux.Handle("/final/objects/batch", batchResponseHandler(t, &received))
			server := httptest.NewServer(mux)
			defer server.Close()

			runBatch(t, server.URL+"/start")

			assert.Equal(t, tc.wantMethod, received.method)
			assert.Equal(t, "Basic test-auth", received.auth)
			assertBatchBody(t, received.body, tc.wantBody)
			if tc.wantBody {
				assert.Equal(t, ClientHeader, received.contentType)
			}
		})
	}
}

func TestBatch_DoesNotRestoreAuthorizationAfterCrossHostRedirect(t *testing.T) {
	var received receivedRequest
	var originalServer *httptest.Server
	originalMux := http.NewServeMux()
	originalMux.Handle("/final/objects/batch", batchResponseHandler(t, &received))
	originalServer = httptest.NewServer(originalMux)
	defer originalServer.Close()

	middleServer := httptest.NewServer(redirectHandler(originalServer.URL+"/final/objects/batch", http.StatusFound))
	defer middleServer.Close()
	originalMux.HandleFunc("/redirect/objects/batch", redirectHandler(middleServer.URL+"/objects/batch", http.StatusFound))

	runBatch(t, originalServer.URL+"/redirect")

	assert.Equal(t, http.MethodPost, received.method)
	assert.Empty(t, received.auth)
	assertBatchBody(t, received.body, true)
	assert.Equal(t, ClientHeader, received.contentType)
}

func TestCheckRedirectFunc_DoesNotRestoreNonReplayablePost(t *testing.T) {
	previous, err := http.NewRequest(http.MethodPost, "https://example.com/start", strings.NewReader("body"))
	require.NoError(t, err)
	previous.GetBody = nil
	pending, err := http.NewRequest(http.MethodGet, "https://example.com/final", nil)
	require.NoError(t, err)
	pending.Response = &http.Response{StatusCode: http.StatusFound}

	err = checkRedirectFunc(pending, []*http.Request{previous})

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, pending.Method)
	assert.Nil(t, pending.Body)
}

func TestCheckRedirectFunc_ReturnsGetBodyError(t *testing.T) {
	for _, tc := range []struct {
		statusCode    int
		pendingMethod string
	}{
		{http.StatusFound, http.MethodGet},
		{http.StatusTemporaryRedirect, http.MethodPost},
	} {
		t.Run(http.StatusText(tc.statusCode), func(t *testing.T) {
			original, err := http.NewRequest(http.MethodPost, "https://example.com/start", strings.NewReader("body"))
			require.NoError(t, err)
			original.GetBody = func() (io.ReadCloser, error) { return nil, assert.AnError }
			previous, err := http.NewRequest(http.MethodPost, "https://example.com/middle", strings.NewReader("different"))
			require.NoError(t, err)
			pending, err := http.NewRequest(tc.pendingMethod, "https://example.com/final", nil)
			require.NoError(t, err)
			pending.Response = &http.Response{StatusCode: tc.statusCode}

			err = checkRedirectFunc(pending, []*http.Request{original, previous})

			require.ErrorIs(t, err, assert.AnError)
			assert.Nil(t, pending.Body)
		})
	}
}

func TestCheckRedirectFunc_ReplaysOriginalBody(t *testing.T) {
	for _, statusCode := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			original, err := http.NewRequest(http.MethodPost, "https://example.com/start", strings.NewReader("original"))
			require.NoError(t, err)
			original.Header.Set("Content-Encoding", "gzip")
			original.Header.Set("Content-Language", "en")
			original.Header.Set("Content-Location", "/start")
			original.Header.Set("Content-Type", ClientHeader)
			previous, err := http.NewRequest(http.MethodPost, "https://example.com/middle", strings.NewReader("different"))
			require.NoError(t, err)
			pending, err := http.NewRequest(http.MethodGet, "https://example.com/final", nil)
			require.NoError(t, err)
			pending.Header.Set("Content-Language", "fr")
			pending.Response = &http.Response{StatusCode: statusCode}
			if statusCode == http.StatusTemporaryRedirect {
				pending.Method = http.MethodPost
			}

			err = checkRedirectFunc(pending, []*http.Request{original, previous})

			require.NoError(t, err)
			body, err := io.ReadAll(pending.Body)
			require.NoError(t, err)
			assert.Equal(t, http.MethodPost, pending.Method)
			assert.Equal(t, "original", string(body))
			assert.Equal(t, original.ContentLength, pending.ContentLength)
			assert.Equal(t, "gzip", pending.Header.Get("Content-Encoding"))
			assert.Equal(t, "fr", pending.Header.Get("Content-Language"))
			assert.Equal(t, "/start", pending.Header.Get("Content-Location"))
			assert.Equal(t, ClientHeader, pending.Header.Get("Content-Type"))
		})
	}
}

func TestCheckRedirectFunc_ReplaysPutOn301And302(t *testing.T) {
	for _, statusCode := range []int{http.StatusMovedPermanently, http.StatusFound} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			original, err := http.NewRequest(http.MethodPut, "https://example.com/start", strings.NewReader("body"))
			require.NoError(t, err)
			pending, err := http.NewRequest(http.MethodGet, "https://example.com/final", nil)
			require.NoError(t, err)
			pending.Response = &http.Response{StatusCode: statusCode}

			err = checkRedirectFunc(pending, []*http.Request{original})

			require.NoError(t, err)
			assert.Equal(t, http.MethodPut, pending.Method)
			assert.Equal(t, original.ContentLength, pending.ContentLength)
			body, err := io.ReadAll(pending.Body)
			require.NoError(t, err)
			assert.Equal(t, "body", string(body))
		})
	}
}

func TestCheckRedirectFunc_DoesNotReplayBodilessGetOrHead(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, statusCode := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
			t.Run(method+" "+http.StatusText(statusCode), func(t *testing.T) {
				retryableRequest, err := retryablehttp.NewRequest(method, "https://example.com/start", nil)
				require.NoError(t, err)
				original := retryableRequest.Request
				original.Header.Set("Content-Encoding", "gzip")
				require.NotNil(t, original.GetBody)
				pending, err := http.NewRequest(method, "https://example.com/final", nil)
				require.NoError(t, err)
				pending.Response = &http.Response{StatusCode: statusCode}

				err = checkRedirectFunc(pending, []*http.Request{original})

				require.NoError(t, err)
				assert.Equal(t, method, pending.Method)
				assert.Nil(t, pending.Body)
				assert.Empty(t, pending.Header.Get("Content-Encoding"))
			})
		}
	}
}

func runBatch(t *testing.T, href string) {
	t.Helper()
	client := newTestClient(t, href)

	_, err := client.Batch("download", []*BatchObject{{Oid: "test-oid", Size: 100}}, "refs/heads/main", "sha256")
	require.NoError(t, err)
}

func newTestClient(t *testing.T, href string) *Client {
	t.Helper()
	client, err := NewClient(&config.Config{}, &commandargs.Shell{}, href, "Basic test-auth")
	require.NoError(t, err)
	return client
}

func batchResponseHandler(t *testing.T, received *receivedRequest) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.method = r.Method
		received.body, _ = io.ReadAll(r.Body)
		received.contentType = r.Header.Get("Content-Type")
		received.auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", ClientHeader)
		assert.NoError(t, json.NewEncoder(w).Encode(&BatchResponse{}))
	})
}

func redirectHandler(location string, statusCode int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, location, statusCode)
	}
}

func assertBatchBody(t *testing.T, body []byte, wantBody bool) {
	t.Helper()
	if !wantBody {
		assert.Empty(t, body)
		return
	}

	var request batchRequest
	require.NoError(t, json.Unmarshal(body, &request))
	assert.Equal(t, "download", request.Operation)
	require.Len(t, request.Objects, 1)
	assert.Equal(t, "test-oid", request.Objects[0].Oid)
}

func TestBatch_EnforcesMaxRedirectLimit(t *testing.T) {
	redirectCount := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		http.Redirect(w, r, server.URL+"/objects/batch", http.StatusFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Batch("download", []*BatchObject{}, "", "sha256")

	require.ErrorContains(t, err, "stopped after 10 redirects")
	assert.Equal(t, 10, redirectCount)
}

func TestLock_FollowsRedirectPreservingPOSTMethod(t *testing.T) {
	var receivedMethod string
	var receivedBody []byte
	var receivedContentType string

	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedBody, _ = io.ReadAll(r.Body)
		receivedContentType = r.Header.Get("Content-Type")

		response := lockResponse{Lock: &Lock{
			ID: "lock1", Path: "/test/file", LockedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Owner: &LockOwner{Name: "testuser"},
		}}
		w.Header().Set("Content-Type", ClientHeader)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(redirectHandler(finalServer.URL+"/locks", http.StatusFound))
	defer redirectServer.Close()

	client := newTestClient(t, redirectServer.URL)

	_, err := client.Lock("/test/file", "refs/heads/main")

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, receivedMethod, "POST method should be preserved after redirect")
	assert.NotEmpty(t, receivedBody, "Request body should be preserved after redirect")
	assert.Equal(t, ClientHeader, receivedContentType)
}

func TestPutObject_FollowsRedirectPreservingPUTMethod(t *testing.T) {
	for _, redirectCode := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(redirectCode), func(t *testing.T) {
			var receivedMethod string
			var receivedBody []byte
			var receivedAuth string

			finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				receivedBody, _ = io.ReadAll(r.Body)
				receivedAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer finalServer.Close()

			redirectServer := httptest.NewServer(redirectHandler(finalServer.URL+"/object", redirectCode))
			defer redirectServer.Close()

			client := newTestClient(t, redirectServer.URL)

			testData := []byte("test file content")
			err := client.PutObject("test-oid", redirectServer.URL+"/object", map[string]string{
				"Authorization": "Basic 1234567890",
			}, io.NopCloser(bytes.NewReader(testData)))

			require.NoError(t, err)
			assert.Equal(t, http.MethodPut, receivedMethod)
			assert.Equal(t, testData, receivedBody)
			assert.Empty(t, receivedAuth)
		})
	}
}

func TestGetObject_FollowsRedirects(t *testing.T) {
	for _, redirectCode := range []int{http.StatusMovedPermanently, http.StatusFound} {
		t.Run(http.StatusText(redirectCode), func(t *testing.T) {
			finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Zero(t, r.ContentLength)
				_, _ = w.Write([]byte("object content"))
			}))
			defer finalServer.Close()

			redirectServer := httptest.NewServer(redirectHandler(finalServer.URL+"/object", redirectCode))
			defer redirectServer.Close()

			client := newTestClient(t, redirectServer.URL)
			body, _, err := client.GetObject("", redirectServer.URL+"/object", nil)
			require.NoError(t, err)
			defer body.Close()
			content, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, "object content", string(content))
		})
	}
}

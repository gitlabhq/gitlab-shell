// Package lfstransfer provides functionality for handling LFS (Large File Storage) transfers.
package lfstransfer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/charmbracelet/git-lfs-transfer/transfer"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

// Client holds configuration, arguments, and authentication details for the client.
type Client struct {
	config *config.Config
	args   *commandargs.Shell
	href   string
	auth   string
	header string
	client *retryablehttp.Client
}

// BatchAction represents an action for a batch operation with metadata.
type BatchAction struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresAt time.Time         `json:"expires_at,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
}

// BatchObject represents an object in a batch operation with its metadata and actions.
type BatchObject struct {
	Oid           string                  `json:"oid,omitempty"`
	Size          int64                   `json:"size"`
	Authenticated bool                    `json:"authenticated,omitempty"`
	Actions       map[string]*BatchAction `json:"actions,omitempty"`
}

type batchRef struct {
	Name string `json:"name,omitempty"`
}

type batchRequest struct {
	Operation     string         `json:"operation"`
	Objects       []*BatchObject `json:"objects"`
	Ref           *batchRef      `json:"ref,omitempty"`
	HashAlgorithm string         `json:"hash_algo,omitempty"`
}

// BatchResponse contains batch operation results and the hash algorithm used.
type BatchResponse struct {
	Objects       []*BatchObject `json:"objects"`
	HashAlgorithm string         `json:"hash_algo,omitempty"`
}

type lockRequest struct {
	Path string    `json:"path"`
	Ref  *batchRef `json:"ref,omitempty"`
}

type lockResponse struct {
	Lock *Lock `json:"lock"`
}

type unlockRequest struct {
	Force bool      `json:"force"`
	Ref   *batchRef `json:"ref,omitempty"`
}

type unlockResponse struct {
	Lock *Lock `json:"lock"`
}

type listLocksVerifyRequest struct {
	Cursor string    `json:"cursor,omitempty"`
	Limit  int       `json:"limit"`
	Ref    *batchRef `json:"ref,omitempty"`
}

// LockOwner represents the owner of a lock.
type LockOwner struct {
	Name string `json:"name"`
}

// Lock represents a lock with its ID, path, timestamp, and owner details.
type Lock struct {
	ID       string     `json:"id"`
	Path     string     `json:"path"`
	LockedAt time.Time  `json:"locked_at"`
	Owner    *LockOwner `json:"owner"`
}

// ListLocksResponse contains a list of locks and a cursor for pagination.
type ListLocksResponse struct {
	Locks      []*Lock `json:"locks,omitempty"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

// ListLocksVerifyResponse provides lists of locks for "ours" and "theirs" with a cursor for pagination.
type ListLocksVerifyResponse struct {
	Ours       []*Lock `json:"ours,omitempty"`
	Theirs     []*Lock `json:"theirs,omitempty"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

// ClientHeader specifies the content type for Git LFS JSON requests.
const ClientHeader = "application/vnd.git-lfs+json"

// NewClient creates a new Client instance using the provided configuration and credentials.
func NewClient(config *config.Config, args *commandargs.Shell, href string, auth string) (*Client, error) {
	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.Logger = nil
	client.HTTPClient.CheckRedirect = checkRedirectFunc

	return &Client{config: config, args: args, href: href, auth: auth, header: ClientHeader, client: client}, nil
}

func (c *Client) newAuthenticatedPostRequest(url string, body io.Reader) (*retryablehttp.Request, error) {
	req, err := retryablehttp.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", c.header)
	req.Header.Set("Authorization", c.auth)

	return req, nil
}

const maxRedirects = 10

func checkRedirectFunc(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}

	original, previous := via[0], via[len(via)-1]
	// net/http turns 301/302 for non-GET/HEAD requests into bodiless GETs and never
	// re-attaches the body on later hops, including 307/308.
	if shouldReplayOriginalRequest(req.Response.StatusCode, original.Method, previous.Method) &&
		req.Body == nil && original.GetBody != nil {
		if err := restoreBody(req, original); err != nil {
			return err
		}
		req.Method = original.Method
	}

	removeAuthorizationAfterCrossHostRedirect(req, via)

	return nil
}

// shouldReplayOriginalRequest replays the original method and body on 301/302/307/308
// until a hop such as 303 changes the method; GET/HEAD have no body to replay.
func shouldReplayOriginalRequest(statusCode int, originalMethod, previousMethod string) bool {
	if originalMethod == http.MethodGet || originalMethod == http.MethodHead || previousMethod != originalMethod {
		return false
	}

	switch statusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func restoreBody(req, original *http.Request) error {
	body, err := original.GetBody()
	if err != nil {
		return err
	}

	req.Body = body
	req.GetBody = original.GetBody
	req.ContentLength = original.ContentLength
	// Go 1.26+ strips body headers when a redirect drops the body; restore them alongside it.
	for _, header := range []string{"Content-Encoding", "Content-Language", "Content-Location", "Content-Type"} {
		if values := original.Header.Values(header); len(req.Header.Values(header)) == 0 && len(values) > 0 {
			req.Header[header] = values
		}
	}

	return nil
}

func removeAuthorizationAfterCrossHostRedirect(req *http.Request, via []*http.Request) {
	host := via[0].URL.Host
	// Unlike net/http, strip Authorization when host:port differs, including subdomains and port changes.
	crossed := req.URL.Host != host || slices.ContainsFunc(via[1:], func(r *http.Request) bool { return r.URL.Host != host })
	if crossed {
		req.Header.Del("Authorization")
	}
}

// Batch performs a batch operation on objects and returns the result.
// The ref parameter is optional and can be an empty string.
func (c *Client) Batch(operation string, reqObjects []*BatchObject, ref string, reqHashAlgo string) (*BatchResponse, error) {
	bref := &batchRef{Name: ref}
	body := batchRequest{
		Operation:     operation,
		Objects:       reqObjects,
		Ref:           bref,
		HashAlgorithm: reqHashAlgo,
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	jsonReader := bytes.NewReader(jsonData)

	req, err := c.newAuthenticatedPostRequest(fmt.Sprintf("%s/objects/batch", c.href), jsonReader)
	if err != nil {
		return nil, err
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	// Error condition taken from example: https://pkg.go.dev/net/http#example-Get
	if res.StatusCode > 399 {
		return nil, fmt.Errorf("response failed with status code: %d", res.StatusCode)
	}

	response := &BatchResponse{}
	if err := gitlabnet.ParseJSON(res, response); err != nil {
		return nil, err
	}

	return response, nil
}

// GetObject performs an HTTP GET request for the object
func (c *Client) GetObject(_, href string, headers map[string]string) (io.ReadCloser, int64, error) {
	req, err := retryablehttp.NewRequest(http.MethodGet, href, nil)
	if err != nil {
		return nil, 0, err
	}
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	// See https://gitlab.com/gitlab-org/gitlab-shell/-/merge_requests/989#note_1891153531 for
	// discussion on bypassing the linter
	res, err := c.client.Do(req) // nolint:bodyclose
	if err != nil {
		return nil, 0, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		_ = res.Body.Close()
		return nil, 0, fs.ErrNotExist
	}

	return res.Body, res.ContentLength, nil
}

// PutObject performs an HTTP PUT request for the object
func (c *Client) PutObject(_, href string, headers map[string]string, r io.Reader) error {
	req, err := retryablehttp.NewRequest(http.MethodPut, href, r)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == 404 {
		return transfer.ErrNotFound
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("internal error (%d)", res.StatusCode)
	}
	return nil
}

// Lock acquires a lock for the specified path with an optional reference name.
func (c *Client) Lock(path, refname string) (*Lock, error) {
	var ref *batchRef
	if refname != "" {
		ref = &batchRef{
			Name: refname,
		}
	}
	body := &lockRequest{
		Path: path,
		Ref:  ref,
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	jsonReader := bytes.NewReader(jsonData)

	req, err := c.newAuthenticatedPostRequest(c.href+"/locks", jsonReader)
	if err != nil {
		return nil, err
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	switch {
	case res.StatusCode >= 200 && res.StatusCode <= 299:
		response := &lockResponse{}
		if err := gitlabnet.ParseJSON(res, response); err != nil {
			return nil, err
		}

		return response.Lock, nil
	case res.StatusCode == http.StatusForbidden:
		return nil, transfer.ErrForbidden
	case res.StatusCode == http.StatusNotFound:
		return nil, transfer.ErrNotFound
	case res.StatusCode == http.StatusConflict:
		response := &lockResponse{}
		if err := gitlabnet.ParseJSON(res, response); err != nil {
			return nil, err
		}

		return response.Lock, transfer.ErrConflict
	default:
		return nil, fmt.Errorf("internal error")
	}
}

// Unlock releases the lock with the given id, optionally forcing the unlock.
func (c *Client) Unlock(id string, force bool, refname string) (*Lock, error) {
	var ref *batchRef
	if refname != "" {
		ref = &batchRef{
			Name: refname,
		}
	}
	body := &unlockRequest{
		Force: force,
		Ref:   ref,
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	jsonReader := bytes.NewReader(jsonData)

	req, err := c.newAuthenticatedPostRequest(fmt.Sprintf("%s/locks/%s/unlock", c.href, id), jsonReader)
	if err != nil {
		return nil, err
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	switch {
	case res.StatusCode >= 200 && res.StatusCode <= 299:
		response := &unlockResponse{}
		if err := gitlabnet.ParseJSON(res, response); err != nil {
			return nil, err
		}

		return response.Lock, nil
	case res.StatusCode == http.StatusForbidden:
		return nil, transfer.ErrForbidden
	case res.StatusCode == http.StatusNotFound:
		return nil, transfer.ErrNotFound
	default:
		return nil, fmt.Errorf("internal error")
	}
}

// ListLocksVerify retrieves locks for the given path and id, with optional pagination.
func (c *Client) ListLocksVerify(path, id, cursor string, limit int, ref string) (*ListLocksVerifyResponse, error) {
	url, err := url.Parse(c.href)
	if err != nil {
		return nil, err
	}
	url = url.JoinPath("locks/verify")
	query := url.Query()
	if path != "" {
		query.Add("path", path)
	}
	if id != "" {
		query.Add("id", id)
	}
	url.RawQuery = query.Encode()

	body := listLocksVerifyRequest{
		Cursor: cursor,
		Limit:  limit,
		Ref: &batchRef{
			Name: ref,
		},
	}
	jsonData, err := json.Marshal(&body)
	if err != nil {
		return nil, err
	}
	jsonReader := bytes.NewReader(jsonData)

	req, err := c.newAuthenticatedPostRequest(url.String(), jsonReader)
	if err != nil {
		return nil, err
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	response := &ListLocksVerifyResponse{}
	if err := gitlabnet.ParseJSON(res, response); err != nil {
		return nil, err
	}

	return response, nil
}

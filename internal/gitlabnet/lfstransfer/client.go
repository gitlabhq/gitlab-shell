package lfstransfer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/charmbracelet/git-lfs-transfer/transfer"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

type Client struct {
	config *config.Config
	args   *commandargs.Shell
	href   string
	auth   string
	header string
}

type BatchAction struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresAt time.Time         `json:"expires_at,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
}

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

type LockOwner struct {
	Name string `json:"name"`
}

type Lock struct {
	ID       string     `json:"id"`
	Path     string     `json:"path"`
	LockedAt time.Time  `json:"locked_at"`
	Owner    *LockOwner `json:"owner"`
}

type ListLocksResponse struct {
	Locks      []*Lock `json:"locks,omitempty"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type ListLocksVerifyResponse struct {
	Ours       []*Lock `json:"ours,omitempty"`
	Theirs     []*Lock `json:"theirs,omitempty"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

var ClientHeader = "application/vnd.git-lfs+json"

func NewClient(config *config.Config, args *commandargs.Shell, href string, auth string) (*Client, error) {
	return &Client{config: config, args: args, href: href, auth: auth, header: ClientHeader}, nil
}
func newHTTPRequest(method string, ref string, reader io.Reader) (*retryablehttp.Request, error) {
	req, err := retryablehttp.NewRequest(method, ref, reader)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func newHTTPClient() *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.Logger = nil
	return client
}

func (c *Client) Batch(operation string, reqObjects []*BatchObject, ref string, reqHashAlgo string) (*BatchResponse, error) {
	// FIXME: This causes tests to fail
	// if ref == "" {
	// 	return nil, errors.New("A ref must be specified.")
	// }

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

	req, _ := newHTTPRequest(http.MethodPost, fmt.Sprintf("%s/objects/batch", c.href), jsonReader)

	req.Header.Set("Content-Type", c.header)
	req.Header.Set("Authorization", c.auth)
	client := newHTTPClient()

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// Error condition taken from example: https://pkg.go.dev/net/http#example-Get
	if res.StatusCode > 399 {
		return nil, fmt.Errorf("response failed with status code: %d", res.StatusCode)
	}

	defer func() { _ = res.Body.Close() }()

	response := &BatchResponse{}
	if err := gitlabnet.ParseJSON(res, response); err != nil {
		return nil, err
	}

	return response, nil
}

func (c *Client) GetObject(oid, href string, headers map[string]string) (io.ReadCloser, int64, error) {
	req, _ := newHTTPRequest(http.MethodGet, href, nil)
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := newHTTPClient()
	// See https://gitlab.com/gitlab-org/gitlab-shell/-/merge_requests/989#note_1891153531 for
	// discussion on bypassing the linter
	res, err := client.Do(req) // nolint:bodyclose
	if err != nil {
		return nil, 0, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, 0, fs.ErrNotExist
	}

	return res.Body, res.ContentLength, nil
}

func (c *Client) PutObject(oid, href string, headers map[string]string, r io.Reader) error {
	req, _ := newHTTPRequest(http.MethodPut, href, r)
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := newHTTPClient()
	res, err := client.Do(req)
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

	req, err := newHTTPRequest(http.MethodPost, c.href+"/locks", jsonReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Authorization", c.auth)

	client := newHTTPClient()
	res, err := client.Do(req)
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

	req, err := newHTTPRequest(http.MethodPost, fmt.Sprintf("%s/locks/%s/unlock", c.href, id), jsonReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Authorization", c.auth)

	client := newHTTPClient()
	res, err := client.Do(req)
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

	req, _ := newHTTPRequest(http.MethodPost, url.String(), jsonReader)
	req.Header.Set("Content-Type", c.header)
	req.Header.Set("Authorization", c.auth)

	client := newHTTPClient()
	res, err := client.Do(req)
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

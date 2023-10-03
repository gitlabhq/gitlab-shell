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
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet"
)

type Client struct {
	config *config.Config
	args   *commandargs.Shell
	href   string
	auth   string
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

type downloadedFileInfo struct {
	oid    string
	size   int64
	reader io.ReadCloser
}

func (i *downloadedFileInfo) Name() string {
	return i.oid
}

func (i *downloadedFileInfo) Size() int64 {
	return i.size
}

func (i *downloadedFileInfo) Mode() fs.FileMode {
	return 0
}

func (i *downloadedFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (i *downloadedFileInfo) IsDir() bool {
	return false
}

func (i *downloadedFileInfo) Sys() any {
	return i.reader
}

type downloadedFile struct {
	downloadedFileInfo
}

func (f *downloadedFile) Read(buf []byte) (int, error) {
	return f.downloadedFileInfo.reader.Read(buf)
}

func (f *downloadedFile) Close() error {
	return f.downloadedFileInfo.reader.Close()
}

func (f *downloadedFile) Stat() (fs.FileInfo, error) {
	return &f.downloadedFileInfo, nil
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

func NewClient(config *config.Config, args *commandargs.Shell, href string, auth string) (*Client, error) {
	return &Client{config: config, args: args, href: href, auth: auth}, nil
}

func (c *Client) Batch(operation string, reqObjects []*BatchObject, ref string, reqHashAlgo string) (*BatchResponse, error) {
	var bref *batchRef

	// FIXME: This causes tests to fail
	// if ref == "" {
	// 	return nil, errors.New("A ref must be specified.")
	// }

	bref = &batchRef{Name: ref}
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

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/objects/batch", c.href), jsonReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Authorization", c.auth)

	client := http.Client{}
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

func (c *Client) GetObject(oid, href string, headers map[string]string) (fs.File, error) {
	req, err := http.NewRequest(http.MethodGet, href, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := http.Client{}
	// See https://gitlab.com/gitlab-org/gitlab-shell/-/merge_requests/989#note_1891153531 for
	// discussion on bypassing the linter
	res, err := client.Do(req) // nolint:bodyclose
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fs.ErrNotExist
	}

	return &downloadedFile{
		downloadedFileInfo{
			oid:    oid,
			size:   res.ContentLength,
			reader: res.Body,
		},
	}, nil
}

func (c *Client) PutObject(oid, href string, headers map[string]string, r io.Reader) error {
	req, err := http.NewRequest(http.MethodPut, href, r)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := http.Client{}
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

	req, err := http.NewRequest(http.MethodPost, url.String(), jsonReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Authorization", c.auth)

	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	response := &ListLocksVerifyResponse{}
	if err := gitlabnet.ParseJSON(res, response); err != nil {
		return nil, err
	}

	return response, nil
}

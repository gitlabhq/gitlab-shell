package lfstransfer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"

	"github.com/charmbracelet/git-lfs-transfer/transfer"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/gitlabnet/lfstransfer"
)

type errCustom struct {
	err     error
	message string
}

func (e *errCustom) Error() string {
	return e.message
}

func (e *errCustom) Is(err error) bool {
	return err == e.err || err == e
}

func newErrUnsupported(operation string) error {
	return &errCustom{
		err:     transfer.ErrNotAllowed,
		message: fmt.Sprintf("%s is not yet supported by git-lfs-transfer. See https://gitlab.com/groups/gitlab-org/-/epics/11872 to track progress.", operation),
	}
}

type GitlabAuthentication struct {
	href string
	auth string
}

type GitlabBackend struct {
	ctx    context.Context
	config *config.Config
	args   *commandargs.Shell
	auth   *GitlabAuthentication
	client *lfstransfer.Client
}

type idData struct {
	Operation string            `json:"operation"`
	Oid       string            `json:"oid"`
	Href      string            `json:"href"`
	Headers   map[string]string `json:"headers,omitempty"`
}

func NewGitlabBackend(ctx context.Context, config *config.Config, args *commandargs.Shell, auth *GitlabAuthentication) (*GitlabBackend, error) {
	client, err := lfstransfer.NewClient(config, args, auth.href, auth.auth)
	if err != nil {
		return nil, err
	}

	return &GitlabBackend{
		ctx,
		config,
		args,
		auth,
		client,
	}, nil
}

func (b *GitlabBackend) issueBatchArgs(op string, oid string, href string, headers map[string]string) (args transfer.Args, err error) {
	data := &idData{
		Operation: op,
		Oid:       oid,
		Href:      href,
		Headers:   headers,
	}

	args = transfer.Args{
		"id":    "",
		"token": "",
	}
	dataBinary, err := json.Marshal(data)
	if err != nil {
		return args, err
	}

	h := hmac.New(sha256.New, []byte(b.config.Secret))
	_, err = h.Write(dataBinary)
	if err != nil {
		return args, err
	}

	args["id"] = base64.StdEncoding.EncodeToString(dataBinary)
	args["token"] = base64.StdEncoding.EncodeToString(h.Sum(nil))

	return args, nil
}

func (b *GitlabBackend) Batch(op string, pointers []transfer.BatchItem, args transfer.Args) ([]transfer.BatchItem, error) {
	reqObjects := make([]*lfstransfer.BatchObject, 0)

	for _, pointer := range pointers {
		reqObject := &lfstransfer.BatchObject{
			Oid:  pointer.Oid,
			Size: pointer.Size,
		}
		reqObjects = append(reqObjects, reqObject)
	}

	refName := args["refname"]
	hashAlgo := args["hash-algo"]

	res, err := b.client.Batch(op, reqObjects, refName, hashAlgo)
	if err != nil {
		return nil, err
	}

	items := make([]transfer.BatchItem, 0)

	for _, retObject := range res.Objects {
		var present bool
		var action *lfstransfer.BatchAction
		var args transfer.Args

		if action, present = retObject.Actions[op]; present {
			args, err = b.issueBatchArgs(op, retObject.Oid, action.Href, action.Header)
			if err != nil {
				return nil, err
			}
		}

		if op == "upload" {
			present = !present
		}

		batchItem := transfer.BatchItem{
			Pointer: transfer.Pointer{
				Oid:  retObject.Oid,
				Size: retObject.Size,
			},
			Present: present,
			Args:    args,
		}
		items = append(items, batchItem)
	}

	return items, nil
}

func (b *GitlabBackend) parseAndCheckBatchArgs(op, oid, id, token string) (href string, headers map[string]string, err error) {
	if id == "" {
		return "", nil, &errCustom{
			err:     transfer.ErrParseError,
			message: "missing id",
		}
	}
	if token == "" {
		return "", nil, &errCustom{
			err:     transfer.ErrUnauthorized,
			message: "missing token",
		}
	}
	idBinary, err := base64.StdEncoding.DecodeString(id)
	if err != nil {
		return "", nil, &errCustom{
			err:     transfer.ErrParseError,
			message: "invalid id",
		}
	}
	tokenBinary, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", nil, &errCustom{
			err:     transfer.ErrParseError,
			message: "invalid token",
		}
	}
	h := hmac.New(sha256.New, []byte(b.config.Secret))
	h.Write(idBinary)
	if !hmac.Equal(tokenBinary, h.Sum(nil)) {
		return "", nil, &errCustom{
			err:     transfer.ErrForbidden,
			message: "token hash mismatch",
		}
	}

	idData := &idData{}
	err = json.Unmarshal(idBinary, idData)
	if err != nil {
		return "", nil, &errCustom{
			err:     transfer.ErrParseError,
			message: "invalid id",
		}
	}
	if idData.Operation != op {
		return "", nil, &errCustom{
			err:     transfer.ErrForbidden,
			message: "invalid operation",
		}
	}
	if idData.Oid != oid {
		return "", nil, &errCustom{
			err:     transfer.ErrForbidden,
			message: "invalid oid",
		}
	}

	return idData.Href, idData.Headers, nil
}

type uploadCloser struct{}

func (c *uploadCloser) Close() error {
	return nil
}

func (b *GitlabBackend) StartUpload(oid string, r io.Reader, args transfer.Args) (io.Closer, error) {
	href, headers, err := b.parseAndCheckBatchArgs("upload", oid, args["id"], args["token"])
	if err != nil {
		io.Copy(io.Discard, r)
		return nil, err
	}
	return &uploadCloser{}, b.client.PutObject(oid, href, headers, r)
}

func (b *GitlabBackend) FinishUpload(state io.Closer, args transfer.Args) error {
	return nil
}

func (b *GitlabBackend) Verify(oid string, args transfer.Args) (transfer.Status, error) {
	return nil, newErrUnsupported("verify-object")
}

func (b *GitlabBackend) Download(oid string, args transfer.Args) (fs.File, error) {
	href, headers, err := b.parseAndCheckBatchArgs("download", oid, args["id"], args["token"])
	if err != nil {
		return nil, err
	}
	return b.client.GetObject(oid, href, headers)
}

func (b *GitlabBackend) LockBackend(args transfer.Args) transfer.LockBackend {
	return &gitlabLockBackend{}
}

type gitlabLock struct {
	*gitlabLockBackend
}

func (l *gitlabLock) Unlock() error {
	return newErrUnsupported("unlock")
}

func (l *gitlabLock) AsArguments() []string {
	return nil
}

func (l *gitlabLock) AsLockSpec(useOwnerID bool) ([]string, error) {
	return nil, nil
}

func (l *gitlabLock) FormattedTimestamp() string {
	return ""
}

func (l *gitlabLock) ID() string {
	return ""
}

func (l *gitlabLock) OwnerName() string {
	return ""
}

func (l *gitlabLock) Path() string {
	return ""
}

type gitlabLockBackend struct{}

func (b *gitlabLockBackend) Create(path string, refname string) (transfer.Lock, error) {
	return nil, newErrUnsupported("lock")
}

func (b *gitlabLockBackend) Unlock(lock transfer.Lock) error {
	return newErrUnsupported("unlock")
}

func (b *gitlabLockBackend) FromPath(path string) (transfer.Lock, error) {
	return &gitlabLock{gitlabLockBackend: b}, nil
}

func (b *gitlabLockBackend) FromID(id string) (transfer.Lock, error) {
	return &gitlabLock{gitlabLockBackend: b}, nil
}

func (b *gitlabLockBackend) Range(cursor string, limit int, iter func(transfer.Lock) error) (string, error) {
	return "", newErrUnsupported("list-lock")
}

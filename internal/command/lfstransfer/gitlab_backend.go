package lfstransfer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

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

func (b *GitlabBackend) Upload(oid string, _ int64, r io.Reader, args transfer.Args) error {
	href, headers, err := b.parseAndCheckBatchArgs("upload", oid, args["id"], args["token"])
	if err != nil {
		_, _ = io.Copy(io.Discard, r)
		return err
	}
	return b.client.PutObject(oid, href, headers, r)
}

func (b *GitlabBackend) Verify(_ string, _ int64, _ transfer.Args) (transfer.Status, error) {
	// Not needed, all verification is done in upload step.
	return transfer.SuccessStatus(), nil
}

func (b *GitlabBackend) Download(oid string, args transfer.Args) (io.ReadCloser, int64, error) {
	href, headers, err := b.parseAndCheckBatchArgs("download", oid, args["id"], args["token"])
	if err != nil {
		return nil, 0, err
	}
	return b.client.GetObject(oid, href, headers)
}

func (b *GitlabBackend) LockBackend(args transfer.Args) transfer.LockBackend {
	return &gitlabLockBackend{
		auth:   b.auth,
		client: b.client,
		args:   args,
	}
}

type gitlabLock struct {
	*gitlabLockBackend
	id        string
	path      string
	timestamp time.Time
	owner     string
	ownerid   string
}

func (l *gitlabLock) Unlock() error {
	lock, err := l.gitlabLockBackend.client.Unlock(l.id, l.gitlabLockBackend.args["force"] == "true", l.gitlabLockBackend.args["refname"])
	if err != nil {
		return err
	}
	l.id = lock.ID
	l.path = lock.Path
	l.timestamp = lock.LockedAt
	if lock.Owner != nil {
		l.owner = lock.Owner.Name
	}
	return nil
}

func (l *gitlabLock) AsArguments() []string {
	return []string{
		fmt.Sprintf("id=%s", l.id),
		fmt.Sprintf("path=%s", l.path),
		fmt.Sprintf("locked-at=%s", l.timestamp.Format(time.RFC3339)),
		fmt.Sprintf("ownername=%s", l.owner),
	}
}

func (l *gitlabLock) AsLockSpec(useOwnerID bool) ([]string, error) {
	spec := []string{
		fmt.Sprintf("lock %s", l.id),
		fmt.Sprintf("path %s %s", l.id, l.path),
		fmt.Sprintf("locked-at %s %s", l.id, l.timestamp.Format(time.RFC3339)),
		fmt.Sprintf("ownername %s %s", l.id, l.owner),
	}
	if useOwnerID {
		spec = append(spec, fmt.Sprintf("owner %s %s", l.id, l.ownerid))
	}
	return spec, nil
}

func (l *gitlabLock) FormattedTimestamp() string {
	return l.timestamp.Format("")
}

func (l *gitlabLock) ID() string {
	return l.id
}

func (l *gitlabLock) OwnerName() string {
	return l.owner
}

func (l *gitlabLock) Path() string {
	return l.path
}

type gitlabLockBackend struct {
	auth   *GitlabAuthentication
	client *lfstransfer.Client
	args   map[string]string
}

func (b *gitlabLockBackend) Create(path string, refname string) (transfer.Lock, error) {
	l, err := b.client.Lock(path, refname)
	var lock *gitlabLock
	if l != nil {
		lock = &gitlabLock{
			gitlabLockBackend: b,
			id:                l.ID,
			path:              l.Path,
			timestamp:         l.LockedAt,
			owner:             l.Owner.Name,
		}
	}
	return lock, err
}

func (b *gitlabLockBackend) Unlock(_ transfer.Lock) error {
	return newErrUnsupported("unlock")
}

func (b *gitlabLockBackend) FromPath(path string) (transfer.Lock, error) {
	res, err := b.client.ListLocksVerify(path, "", "", 1, "")
	if err != nil {
		return nil, err
	}
	var lock *lfstransfer.Lock
	var owner string
	switch {
	case len(res.Ours) == 1 && len(res.Theirs) == 0:
		lock = res.Ours[0]
		owner = "ours"
	case len(res.Ours) == 0 && len(res.Theirs) == 1:
		lock = res.Theirs[0]
		owner = "theirs"
	case len(res.Ours) == 0 && len(res.Theirs) == 0:
		return nil, nil
	default:
		return nil, errors.New("internal error")
	}
	return &gitlabLock{
		gitlabLockBackend: b,
		id:                lock.ID,
		path:              lock.Path,
		timestamp:         lock.LockedAt,
		owner:             lock.Owner.Name,
		ownerid:           owner,
	}, nil
}

func (b *gitlabLockBackend) FromID(id string) (transfer.Lock, error) {
	return &gitlabLock{
		gitlabLockBackend: b,
		id:                id,
	}, nil
}

func (b *gitlabLockBackend) Range(cursor string, limit int, iter func(transfer.Lock) error) (string, error) {
	res, err := b.client.ListLocksVerify(b.args["path"], b.args["id"], cursor, limit, b.args["refname"])
	if err != nil {
		return "", err
	}
	for _, lock := range res.Ours {
		tlock := &gitlabLock{
			gitlabLockBackend: b,
			id:                lock.ID,
			path:              lock.Path,
			timestamp:         lock.LockedAt,
			owner:             lock.Owner.Name,
			ownerid:           "ours",
		}
		err = iter(tlock)
		if err != nil {
			return "", err
		}
	}
	for _, lock := range res.Theirs {
		tlock := &gitlabLock{
			gitlabLockBackend: b,
			id:                lock.ID,
			path:              lock.Path,
			timestamp:         lock.LockedAt,
			owner:             lock.Owner.Name,
			ownerid:           "theirs",
		}
		err = iter(tlock)
		if err != nil {
			return "", err
		}
	}

	return res.NextCursor, nil
}

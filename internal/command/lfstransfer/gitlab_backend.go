package lfstransfer

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/charmbracelet/git-lfs-transfer/transfer"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/command/commandargs"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/config"
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
}

func NewGitlabBackend(ctx context.Context, config *config.Config, args *commandargs.Shell, auth *GitlabAuthentication) (*GitlabBackend, error) {
	return &GitlabBackend{
		ctx,
		config,
		args,
		auth,
	}, nil
}

func (b *GitlabBackend) Batch(op string, pointers []transfer.BatchItem, args transfer.Args) ([]transfer.BatchItem, error) {
	return nil, newErrUnsupported("batch")
}

func (b *GitlabBackend) StartUpload(oid string, r io.Reader, args transfer.Args) (io.Closer, error) {
	io.Copy(io.Discard, r)
	return nil, newErrUnsupported("put-object")
}

func (b *GitlabBackend) FinishUpload(state io.Closer, args transfer.Args) error {
	return newErrUnsupported("put-object")
}

func (b *GitlabBackend) Verify(oid string, args transfer.Args) (transfer.Status, error) {
	return nil, newErrUnsupported("verify-object")
}

func (b *GitlabBackend) Download(oid string, args transfer.Args) (fs.File, error) {
	return nil, newErrUnsupported("get-object")
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

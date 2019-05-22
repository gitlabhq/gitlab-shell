package fallback

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeExec struct {
	OldExec func(string, []string, []string) error
	Error   error
	Called  bool

	Filename string
	Args     []string
	Env      []string
}

var (
	fakeArgs = []string{"./test", "foo", "bar"}
)

func (f *fakeExec) Exec(filename string, args []string, env []string) error {
	f.Called = true

	f.Filename = filename
	f.Args = args
	f.Env = env

	return f.Error
}

func (f *fakeExec) Setup() {
	f.OldExec = execFunc
	execFunc = f.Exec
}

func (f *fakeExec) Cleanup() {
	execFunc = f.OldExec
}

func TestExecuteExecsCommandSuccesfully(t *testing.T) {
	cmd := &Command{RootDir: "/tmp", Args: fakeArgs}

	// Override the exec func
	fake := &fakeExec{}
	fake.Setup()
	defer fake.Cleanup()

	require.NoError(t, cmd.Execute())
	require.True(t, fake.Called)
	require.Equal(t, fake.Filename, "/tmp/bin/gitlab-shell-ruby")
	require.Equal(t, fake.Args, []string{"/tmp/bin/gitlab-shell-ruby", "foo", "bar"})
	require.Equal(t, fake.Env, os.Environ())
}

func TestExecuteExecsCommandOnError(t *testing.T) {
	cmd := &Command{RootDir: "/test", Args: fakeArgs}

	// Override the exec func
	fake := &fakeExec{Error: errors.New("Test error")}
	fake.Setup()
	defer fake.Cleanup()

	require.Error(t, cmd.Execute())
	require.True(t, fake.Called)
}

func TestExecuteGivenNonexistentCommand(t *testing.T) {
	cmd := &Command{RootDir: "/tmp/does/not/exist", Args: fakeArgs}

	require.Error(t, cmd.Execute())
}

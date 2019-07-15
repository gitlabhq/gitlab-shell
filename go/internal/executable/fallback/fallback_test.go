package fallback

import (
	"errors"
	"fmt"
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

func TestExecute(t *testing.T) {
	// Override the exec func
	fake := &fakeExec{}
	fake.Setup()
	defer fake.Cleanup()

	tests := []struct {
		name       string
		executable *Executable
	}{
		{
			name:       "default (no Program set)",
			executable: &Executable{RootDir: "/tmp", Args: fakeArgs},
		},
		{
			name:       "Program is set",
			executable: &Executable{Program: "program", RootDir: "/tmp", Args: fakeArgs},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.executable.Execute())
			require.True(t, fake.Called)
			require.Equal(t, fake.Filename, fmt.Sprintf("/tmp/bin/%s", tt.executable.Program))
			require.Equal(t, fake.Args, []string{fake.Filename, "foo", "bar"})
			require.Equal(t, fake.Env, os.Environ())
		})
	}
}

func TestExecuteFailure(t *testing.T) {
	// Override the exec func
	fake := &fakeExec{Error: errors.New("Test error")}
	fake.Setup()
	defer fake.Cleanup()

	tests := []struct {
		name       string
		executable *Executable
	}{
		{
			name:       "default (no Program set)",
			executable: &Executable{RootDir: "/test", Args: fakeArgs},
		},
		{
			name:       "Program is set",
			executable: &Executable{Program: "program", RootDir: "/test", Args: fakeArgs},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.executable.Execute())
			require.True(t, fake.Called)
		})
	}
}

func TestExecuteGivenNonexistent(t *testing.T) {
	tests := []struct {
		name       string
		executable *Executable
	}{
		{
			name:       "default (no Program set)",
			executable: &Executable{RootDir: "/tmp/does/not/exist", Args: fakeArgs},
		},
		{
			name:       "Program is set",
			executable: &Executable{Program: "program", RootDir: "/tmp/does/not/exist", Args: fakeArgs},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.executable.Execute())
		})
	}
}

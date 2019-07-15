package executable

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeOs struct {
	OldExecutable func() (string, error)
	Path          string
	Error         error
	Called        bool
}

func (f *fakeOs) Executable() (string, error) {
	f.Called = true

	return f.Path, f.Error
}

func (f *fakeOs) Setup() {
	f.OldExecutable = osExecutable
	osExecutable = f.Executable
}

func (f *fakeOs) Cleanup() {
	osExecutable = f.OldExecutable
}

func TestNew(t *testing.T) {
	// Override the os.Executable
	fake := &fakeOs{Path: "/tmp/gitlab-shell"}
	fake.Setup()
	defer fake.Cleanup()

	executable, err := New()

	require.NoError(t, err)
	require.Equal(t, executable.Name, "gitlab-shell")
}

func TestFailingNew(t *testing.T) {
	// Override the os.Executable
	fake := &fakeOs{Path: "/tmp/gitlab-shell", Error: errors.New("Test error")}
	fake.Setup()
	defer fake.Cleanup()

	_, err := New()

	require.Error(t, err)
	require.True(t, fake.Called)
}

func TestIsForExecutingCommand(t *testing.T) {
	testCases := []struct {
		desc        string
		executable  *Executable
		expectation bool
	}{
		{
			desc:        "Name is not gitlab-shell",
			executable:  &Executable{Name: "gitlab-shell-authorized-keys-check"},
			expectation: false,
		},
		{
			desc:        "Name is gitlab-shell",
			executable:  &Executable{Name: "gitlab-shell"},
			expectation: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			require.Equal(t, tc.executable.IsForExecutingCommand(), tc.expectation)
		})
	}
}

func TestFallbackCommand(t *testing.T) {
	executable := &Executable{Name: "gitlab-shell"}

	require.Equal(t, executable.FallbackProgram(), "gitlab-shell-ruby")
}

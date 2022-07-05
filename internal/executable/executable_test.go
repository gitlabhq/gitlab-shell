package executable

import (
	"errors"
	"testing"

	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/testhelper"

	"github.com/stretchr/testify/require"
)

type fakeOs struct {
	OldExecutable func() (string, error)
	Path          string
	Error         error
}

func (f *fakeOs) Executable() (string, error) {
	return f.Path, f.Error
}

func (f *fakeOs) Setup() {
	f.OldExecutable = osExecutable
	osExecutable = f.Executable
}

func (f *fakeOs) Cleanup() {
	osExecutable = f.OldExecutable
}

func TestNewSuccess(t *testing.T) {
	testCases := []struct {
		desc            string
		fakeOs          *fakeOs
		environment     map[string]string
		expectedRootDir string
	}{
		{
			desc:            "GITLAB_SHELL_DIR env var is not defined",
			fakeOs:          &fakeOs{Path: "/tmp/bin/gitlab-shell"},
			expectedRootDir: "/tmp",
		},
		{
			desc:   "GITLAB_SHELL_DIR env var is defined",
			fakeOs: &fakeOs{Path: "/opt/bin/gitlab-shell"},
			environment: map[string]string{
				"GITLAB_SHELL_DIR": "/tmp",
			},
			expectedRootDir: "/tmp",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			fake := tc.fakeOs
			fake.Setup()
			defer fake.Cleanup()

			result, err := New("gitlab-shell")

			require.NoError(t, err)
			require.Equal(t, result.Name, "gitlab-shell")
			require.Equal(t, result.RootDir, tc.expectedRootDir)
		})
	}
}

func TestNewFailure(t *testing.T) {
	testCases := []struct {
		desc        string
		fakeOs      *fakeOs
		environment map[string]string
	}{
		{
			desc:   "failed to determine executable",
			fakeOs: &fakeOs{Path: "", Error: errors.New("error")},
		},
		{
			desc:   "GITLAB_SHELL_DIR doesn't exist",
			fakeOs: &fakeOs{Path: "/tmp/bin/gitlab-shell"},
			environment: map[string]string{
				"GITLAB_SHELL_DIR": "/tmp/non/existing/directory",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			restoreEnv := testhelper.TempEnv(tc.environment)
			defer restoreEnv()

			fake := tc.fakeOs
			fake.Setup()
			defer fake.Cleanup()

			_, err := New("gitlab-shell")

			require.Error(t, err)
		})
	}
}

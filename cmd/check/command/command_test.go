package command_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/cmd/check/command"
	"gitlab.com/gitlab-org/gitlab-shell/internal/command/healthcheck"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
	"gitlab.com/gitlab-org/gitlab-shell/internal/executable"
	"gitlab.com/gitlab-org/gitlab-shell/internal/sshenv"
)

var (
	basicConfig = &config.Config{GitlabUrl: "http+unix://gitlab.socket"}
)

func TestNew(t *testing.T) {
	testCases := []struct {
		desc         string
		executable   *executable.Executable
		env          sshenv.Env
		arguments    []string
		config       *config.Config
		expectedType interface{}
	}{
		{
			desc:         "it returns a Healthcheck command",
			config:       basicConfig,
			expectedType: &healthcheck.Command{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			command, err := command.New(tc.config, nil)

			require.NoError(t, err)
			require.IsType(t, tc.expectedType, command)
		})
	}
}

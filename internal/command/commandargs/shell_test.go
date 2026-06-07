package commandargs

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/v14/internal/topology"
)

const (
	testUsername      = "jane-doe"
	testKrb5Principal = "jane@EXAMPLE.COM"
)

func TestUserArgs(t *testing.T) {
	tests := []struct {
		name     string
		shell    Shell
		expected topology.UserArgs
	}{
		{
			name: "all fields populated",
			shell: Shell{
				GitlabUsername:      testUsername,
				GitlabKeyID:         "123",
				GitlabKrb5Principal: testKrb5Principal,
			},
			expected: topology.UserArgs{
				Username:      testUsername,
				KeyID:         "123",
				Krb5Principal: testKrb5Principal,
			},
		},
		{
			name:     "only username",
			shell:    Shell{GitlabUsername: testUsername},
			expected: topology.UserArgs{Username: testUsername},
		},
		{
			name:     "only key ID",
			shell:    Shell{GitlabKeyID: "123"},
			expected: topology.UserArgs{KeyID: "123"},
		},
		{
			name:     "only krb5 principal",
			shell:    Shell{GitlabKrb5Principal: testKrb5Principal},
			expected: topology.UserArgs{Krb5Principal: testKrb5Principal},
		},
		{
			name:     "empty shell",
			shell:    Shell{},
			expected: topology.UserArgs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.shell.UserArgs())
		})
	}
}

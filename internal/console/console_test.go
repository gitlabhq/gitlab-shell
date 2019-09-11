package console

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisplayWarningMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantOut string
	}{
		{
			name:    "empty",
			message: "",
			wantOut: "",
		},
		{
			name:    "basically empty",
			message: " ",
			wantOut: "",
		},
		{
			name:    "something",
			message: "something",
			wantOut: `remote: 
remote: ========================================================================
remote: 
remote: something
remote: 
remote: ========================================================================
remote: 
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			DisplayWarningMessage(tt.message, out)

			require.Equal(t, tt.wantOut, out.String())
		})
	}
}

func TestDisplayWarningMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		wantOut  string
	}{
		{
			name:     "empty",
			messages: []string{""},
			wantOut:  "",
		},
		{
			name:     "basically empty",
			messages: []string{" "},
			wantOut:  "",
		},
		{
			name:     "something",
			messages: []string{"something", "here"},
			wantOut: `remote: 
remote: ========================================================================
remote: 
remote: something
remote: here
remote: 
remote: ========================================================================
remote: 
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			DisplayWarningMessages(tt.messages, out)

			require.Equal(t, tt.wantOut, out.String())
		})
	}
}

func TestDisplayInfoMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantOut string
	}{
		{
			name:    "empty",
			message: "",
			wantOut: "",
		},
		{
			name:    "basically empty",
			message: " ",
			wantOut: "",
		},
		{
			name:    "something",
			message: "something",
			wantOut: `remote: 
remote: something
remote: 
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			DisplayInfoMessage(tt.message, out)

			require.Equal(t, tt.wantOut, out.String())
		})
	}
}

func TestDisplayInfoMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		wantOut  string
	}{
		{
			name:     "empty",
			messages: []string{""},
			wantOut:  "",
		},
		{
			name:     "basically empty",
			messages: []string{" "},
			wantOut:  "",
		},
		{
			name:     "something",
			messages: []string{"something", "here"},
			wantOut:  "remote: \nremote: something\nremote: here\nremote: \n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			DisplayInfoMessages(tt.messages, out)

			require.Equal(t, tt.wantOut, out.String())
		})
	}
}

func Test_noMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		want     bool
	}{
		{
			name:     "empty",
			messages: []string{""},
			want:     true,
		},
		{
			name:     "basically empty",
			messages: []string{" "},
			want:     true,
		},
		{
			name:     "something",
			messages: []string{"something", "here"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, noMessages(tt.messages))
		})
	}
}

func Test_formatLine(t *testing.T) {
	require.Equal(t, "remote: something\n", formatLine("something"))
}

func Test_divider(t *testing.T) {
	want := `remote: 
remote: ========================================================================
remote: 
`

	require.Equal(t, want, divider())
}

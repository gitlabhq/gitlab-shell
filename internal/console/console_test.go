package console

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testNameEmpty          = "empty"
	testNameBasicallyEmpty = "basically empty"
	testNameSomething      = "something"
	testNameHere           = "here"
)

func TestDisplayWarningMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantOut string
	}{
		{
			name:    testNameEmpty,
			message: "",
			wantOut: "",
		},
		{
			name:    testNameBasicallyEmpty,
			message: " ",
			wantOut: "",
		},
		{
			name:    testNameSomething,
			message: testNameSomething,
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
			name:     testNameEmpty,
			messages: []string{""},
			wantOut:  "",
		},
		{
			name:     testNameBasicallyEmpty,
			messages: []string{" "},
			wantOut:  "",
		},
		{
			name:     testNameSomething,
			messages: []string{testNameSomething, testNameHere},
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
			name:    testNameEmpty,
			message: "",
			wantOut: "",
		},
		{
			name:    testNameBasicallyEmpty,
			message: " ",
			wantOut: "",
		},
		{
			name:    testNameSomething,
			message: testNameSomething,
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
			name:     testNameEmpty,
			messages: []string{""},
			wantOut:  "",
		},
		{
			name:     testNameBasicallyEmpty,
			messages: []string{" "},
			wantOut:  "",
		},
		{
			name:     testNameSomething,
			messages: []string{testNameSomething, testNameHere},
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
			name:     testNameEmpty,
			messages: []string{""},
			want:     true,
		},
		{
			name:     testNameBasicallyEmpty,
			messages: []string{" "},
			want:     true,
		},
		{
			name:     testNameSomething,
			messages: []string{testNameSomething, testNameHere},
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
	require.Equal(t, "remote: something\n", formatLine(testNameSomething))
}

func Test_divider(t *testing.T) {
	want := `remote: 
remote: ========================================================================
remote: 
`

	require.Equal(t, want, divider())
}

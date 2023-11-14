package readwriter

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountingWriter_Write(t *testing.T) {
	testString := []byte("test string")
	buffer := &bytes.Buffer{}

	cw := &CountingWriter{
		W: buffer,
	}

	n, err := cw.Write(testString)

	require.NoError(t, err)
	require.Equal(t, 11, n)
	require.Equal(t, int64(11), cw.N)

	cw.Write(testString)
	require.Equal(t, int64(22), cw.N)
}

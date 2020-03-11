package logger

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitlab-shell/internal/config"
)

func TestConfigure(t *testing.T) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "logtest-")
	require.NoError(t, err)
	defer tmpFile.Close()

	config := config.Config{
		LogFile:   tmpFile.Name(),
		LogFormat: "json",
	}

	err = Configure(&config)

	require.NoError(t, err)

	log.Info("this is a test")

	tmpFile.Close()

	data, err := ioutil.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.True(t, strings.Contains(string(data), `msg":"this is a test"`))
}

func TestElapsedTimeMs(t *testing.T) {
	testCases := []struct {
		delta    float64
		expected float64
	}{
		{
			delta:    123.0,
			expected: 123.0,
		},
		{
			delta:    123.4,
			expected: 123.4,
		},
		{
			delta:    123.45,
			expected: 123.45,
		},
		{
			delta:    123.456,
			expected: 123.456,
		},

		{
			delta:    123.4567,
			expected: 123.457,
		},
		{
			delta:    123.4564,
			expected: 123.456,
		},
	}

	for _, tc := range testCases {
		duration := fmt.Sprintf("%fms", tc.delta)

		t.Run(duration, func(t *testing.T) {
			delta, _ := time.ParseDuration(duration)
			start := time.Now()
			end := start.Add(delta)
			require.Equal(t, tc.expected, ElapsedTimeMs(start, end))
			require.InDelta(t, tc.expected, ElapsedTimeMs(start, end), 0.001)
		})
	}
}

package gitlab

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNew_ReadTimeout(t *testing.T) {
	cases := []struct {
		name               string
		readTimeoutSeconds uint64
		wantTimeout        time.Duration
	}{
		{"default", 0, 300 * time.Second},
		{"configured", 10, 10 * time.Second},
		{"overflow", math.MaxUint64, 300 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(&Config{
				GitlabURL:          "http://localhost",
				Secret:             "test-secret",
				ReadTimeoutSeconds: tc.readTimeoutSeconds,
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantTimeout, c.inner.HTTPClient().Timeout)
		})
	}
}

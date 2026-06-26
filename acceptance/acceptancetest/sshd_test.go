//go:build acceptance

package acceptancetest

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartSSHD_boots(t *testing.T) {
	d := StartSSHD(t, SSHDConfig{
		InternalAPIURL: "http://127.0.0.1:1", // unused; daemon only needs it to boot
		Secret:         "test-secret",
	})

	conn, err := net.DialTimeout("tcp", d.Addr, 2*time.Second)
	require.NoError(t, err, "daemon should be accepting TCP connections")
	_ = conn.Close()
}

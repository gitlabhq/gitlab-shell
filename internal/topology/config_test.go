package topology

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	t.Run("disabled config is always valid", func(t *testing.T) {
		require.NoError(t, (&Config{Enabled: false}).Validate())
		require.NoError(t, (&Config{Enabled: false, Address: ""}).Validate())
	})

	t.Run("enabled config requires address", func(t *testing.T) {
		err := (&Config{Enabled: true}).Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "address is required")
	})

	t.Run("enabled config requires address in host:port format", func(t *testing.T) {
		err := (&Config{Enabled: true, Address: "localhost"}).Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be in host:port format")
	})

	t.Run("enabled config with address is valid", func(t *testing.T) {
		cfg := &Config{Enabled: true, Address: "localhost:8080"}
		require.NoError(t, cfg.Validate())
	})

	t.Run("enabled config with TLS is valid", func(t *testing.T) {
		cfg := &Config{
			Enabled: true,
			Address: "topology.gitlab.com:443",
			TLS:     TLSConfig{Enabled: true, CAFile: "/path/to/ca.crt"},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("invalid classify_type fails", func(t *testing.T) {
		cfg := &Config{Enabled: true, Address: "localhost:8080", ClassifyType: "invalid_type"}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid topology_service.classify_type")
	})

	t.Run("valid classify_types succeed", func(t *testing.T) {
		for _, ct := range []string{"first_cell", "session_prefix", "cell_id", ""} {
			cfg := &Config{Enabled: true, Address: "localhost:8080", ClassifyType: ct}
			require.NoError(t, cfg.Validate(), "classify_type=%q should be valid", ct)
		}
	})
}

func TestTLSConfigValidate(t *testing.T) {
	t.Run("disabled TLS is always valid", func(t *testing.T) {
		require.NoError(t, (&TLSConfig{Enabled: false}).Validate())
	})

	t.Run("enabled TLS without CA uses system CAs", func(t *testing.T) {
		require.NoError(t, (&TLSConfig{Enabled: true}).Validate())
	})

	t.Run("mTLS requires both cert and key", func(t *testing.T) {
		err := (&TLSConfig{Enabled: true, CertFile: "/cert.crt"}).Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "both cert_file and key_file must be provided")

		err = (&TLSConfig{Enabled: true, KeyFile: "/key.pem"}).Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "both cert_file and key_file must be provided")
	})

	t.Run("mTLS with both cert and key is valid", func(t *testing.T) {
		cfg := &TLSConfig{Enabled: true, CertFile: "/cert.crt", KeyFile: "/key.pem"}
		require.NoError(t, cfg.Validate())
	})

	t.Run("full mTLS config is valid", func(t *testing.T) {
		cfg := &TLSConfig{
			Enabled:    true,
			CAFile:     "/ca.crt",
			CertFile:   "/cert.crt",
			KeyFile:    "/key.pem",
			ServerName: "topology.gitlab.com",
		}
		require.NoError(t, cfg.Validate())
	})
}

func TestValidClassifyTypes(t *testing.T) {
	expected := []string{"first_cell", "session_prefix", "cell_id"}
	require.Equal(t, expected, ValidClassifyTypes)
}

func TestDefaultTimeout(t *testing.T) {
	require.Equal(t, 5*time.Second, DefaultTimeout)
}

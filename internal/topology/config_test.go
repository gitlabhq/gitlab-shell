package topology

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testTopologyAddress = "localhost:8080"

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
		cfg := &Config{
			Enabled:      true,
			Address:      testTopologyAddress,
			CellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("enabled config with TLS is valid", func(t *testing.T) {
		cfg := &Config{
			Enabled:      true,
			Address:      "topology.gitlab.com:443",
			TLS:          TLSConfig{Enabled: true, CAFile: "/path/to/ca.crt"},
			CellEndpoint: CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("enabled config without cell_endpoint is invalid", func(t *testing.T) {
		err := (&Config{Enabled: true, Address: testTopologyAddress}).Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "cell_endpoint")
		require.Contains(t, err.Error(), "scheme is required")
	})

	t.Run("disabled config with empty cell_endpoint is valid", func(t *testing.T) {
		cfg := &Config{Enabled: false, CellEndpoint: CellEndpointConfig{}}
		require.NoError(t, cfg.Validate())
	})
}

func TestCellEndpointConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     CellEndpointConfig
		wantErr bool
	}{
		{"http scheme valid", CellEndpointConfig{Scheme: schemeHTTP, Port: 80}, false},
		{"https scheme valid", CellEndpointConfig{Scheme: schemeHTTPS, Port: 8181}, false},
		{"ftp scheme invalid", CellEndpointConfig{Scheme: "ftp", Port: 8181}, true},
		{"empty scheme invalid", CellEndpointConfig{Scheme: "", Port: 8181}, true},
		{"port 1 valid", CellEndpointConfig{Scheme: schemeHTTPS, Port: 1}, false},
		{"port 65535 valid", CellEndpointConfig{Scheme: schemeHTTPS, Port: 65535}, false},
		{"port 0 invalid", CellEndpointConfig{Scheme: schemeHTTPS, Port: 0}, true},
		{"port 65536 invalid", CellEndpointConfig{Scheme: schemeHTTPS, Port: 65536}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
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

func TestDefaultTimeout(t *testing.T) {
	require.Equal(t, 5*time.Second, DefaultTimeout)
}

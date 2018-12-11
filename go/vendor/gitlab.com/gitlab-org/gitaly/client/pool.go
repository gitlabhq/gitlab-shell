// +build !darwin

package client

import "crypto/x509"

// systemCertPool has an override on macOS.
func systemCertPool() (*x509.CertPool, error) { return x509.SystemCertPool() }

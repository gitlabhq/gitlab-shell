// +build darwin

package client

import (
	"crypto/x509"
	"io/ioutil"
	"os"
	"path"
)

// systemCertPool circumvents the fact that Go on macOS does not support
// SSL_CERT_{DIR,FILE}.
func systemCertPool() (*x509.CertPool, error) {
	var certPem []byte
	count := 0

	if f := os.Getenv("SSL_CERT_FILE"); len(f) > 0 {
		pem, err := ioutil.ReadFile(f)
		if err != nil {
			return nil, err
		}

		pem = append(pem, '\n')
		certPem = append(certPem, pem...)
		count++
	}

	if d := os.Getenv("SSL_CERT_DIR"); len(d) > 0 {
		entries, err := ioutil.ReadDir(d)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			pem, err := ioutil.ReadFile(path.Join(d, entry.Name()))
			if err != nil {
				return nil, err
			}

			pem = append(pem, '\n')
			certPem = append(certPem, pem...)
			count++
		}
	}

	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}

	pool.AppendCertsFromPEM(certPem)
	return pool, nil
}

//go:build boringcrypto
// +build boringcrypto

package boring

import (
	"crypto/boring"

	"gitlab.com/gitlab-org/labkit/log"
)

// CheckBoring checks whether FIPS crypto has been enabled. For the FIPS Go
// compiler in https://github.com/golang-fips/go, this requires that:
//
// 1. The kernel has FIPS enabled (e.g. `/proc/sys/crypto/fips_enabled` is 1).
// 2. A system OpenSSL can be dynamically loaded via ldopen().
func CheckBoring() {
	if boring.Enabled() {
		log.Info("FIPS mode is enabled. Using an external SSL library.")
		return
	}
	log.Info("Gitaly was compiled with FIPS mode, but an external SSL library was not enabled.")
}

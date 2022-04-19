//go:build !boringcrypto
// +build !boringcrypto

package boring

// CheckBoring does nothing when the boringcrypto tag is not in the
// build.
func CheckBoring() {
}

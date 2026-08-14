// Command genhostkey writes throwaway OpenSSH host keys for the ssh-audit
// harness (support/ssh-audit/run.sh). It exists so the harness does not depend
// on ssh-keygen, which is not installed in all CI images (notably the FIPS UBI
// image). Keys are written as $dir/ssh_host_<type>_key.
package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

func main() {
	dir := flag.String("dir", "", "directory to write host keys into")
	types := flag.String("types", "rsa ecdsa ed25519", "space-separated host key types")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "error: -dir is required")
		os.Exit(2)
	}

	for keyType := range strings.FieldsSeq(*types) {
		if err := writeHostKey(*dir, keyType); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
}

func writeHostKey(dir, keyType string) error {
	key, err := generateKey(keyType)
	if err != nil {
		return err
	}

	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		return fmt.Errorf("marshal %s host key: %w", keyType, err)
	}

	path := filepath.Join(dir, "ssh_host_"+keyType+"_key")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// generateKey returns a private key whose sizes match those pinned in the
// committed policies (RSA 4096, ECDSA P-256).
func generateKey(keyType string) (crypto.PrivateKey, error) {
	switch keyType {
	case "rsa":
		return rsa.GenerateKey(rand.Reader, 4096)
	case "ecdsa":
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ed25519":
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	default:
		return nil, fmt.Errorf("unsupported host key type %q", keyType)
	}
}

//go:build !windows

package security

import "os"

func protectSecret(path string) error { return os.Chmod(path, 0600) }

func syncSecretDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

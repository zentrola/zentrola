//go:build !windows

package logging

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func acquireFileLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: %s", ErrFileInUse, path)
		}
		return nil, err
	}
	return file, nil
}

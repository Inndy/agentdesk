//go:build linux || darwin

package desk

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func LockServer(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, "server.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another service owns this project: %w", err)
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

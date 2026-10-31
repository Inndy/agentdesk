package desk

import (
	"os"
	"path/filepath"
)

// WriteServerPID publishes a conventional decimal PID file. The caller must
// hold LockServer until after cleanup, including on startup failure.
// The persistent OS lock remains authoritative: stale or reused PIDs do not
// determine whether a new process may start.
func WriteServerPID(dir string) (func(), error) {
	path := filepath.Join(dir, "server.pid")
	if err := WriteJSON(path, os.Getpid()); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

package desk

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestServerPIDReplacesStaleFileAndCleansUpUnderLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.pid")
	if err := os.WriteFile(path, []byte("999999999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unlock, err := LockServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	cleanup, err := WriteServerPID(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
		t.Fatal("PID file does not identify current owner")
	}
	if other, err := LockServer(dir); err == nil {
		other()
		t.Fatal("PID file allowed a second owner")
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("PID file was not removed: %v", err)
	}
	if other, err := LockServer(dir); err == nil {
		other()
		t.Fatal("cleanup released the lock before shutdown completed")
	}
}

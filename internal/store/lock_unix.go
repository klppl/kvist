//go:build unix

package store

import (
	"os"
	"syscall"
)

// fileLock is an exclusive advisory lock that also excludes other processes
// (the CLI and a running server share the data directory).
type fileLock struct{ f *os.File }

func lockFile(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return &fileLock{f}, nil
}

func (l *fileLock) unlock() {
	syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	l.f.Close()
}

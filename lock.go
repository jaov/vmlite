package main

import (
	"fmt"
	"os"
	"syscall"
)

type projectLock struct {
	file *os.File
}

func acquireProjectLock(layout projectLayout) (*projectLock, error) {
	if layout.LockFile == "" {
		return nil, fmt.Errorf("state lock path is empty")
	}
	if err := os.MkdirAll(layout.StateDir, 0o755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	f, err := os.OpenFile(layout.LockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open state lock %q: %w", layout.LockFile, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, fmt.Errorf("VM state is locked by another vmlite process (%s)", layout.LockFile)
		}
		return nil, fmt.Errorf("lock state %q: %w", layout.LockFile, err)
	}
	return &projectLock{file: f}, nil
}

func (l *projectLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}

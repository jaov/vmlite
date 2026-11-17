package state

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireLockExcludesSecondProcess(t *testing.T) {
	dir := t.TempDir()
	layout := ProjectLayout{
		StateDir: dir,
		LockFile: filepath.Join(dir, "vm.lock"),
	}
	first, err := AcquireLock(layout)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if _, err := AcquireLock(layout); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("second lock = %v, want locked error", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	second, err := AcquireLock(layout)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second: %v", err)
	}
}

package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledVmlitePathPrefix(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "vmlite")
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write vmlite: %v", err)
	}
	got, err := installedVmlitePath(dir)
	if err != nil {
		t.Fatalf("installedVmlitePath: %v", err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestInstalledVmlitePathMissingPrefix(t *testing.T) {
	dir := t.TempDir()
	if _, err := installedVmlitePath(dir); err == nil {
		t.Fatal("expected missing binary error")
	}
}

func TestInstalledVmlitePathNotInPATH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := installedVmlitePath(""); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestInstalledVmlitePathFromPATH(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "vmlite")
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write vmlite: %v", err)
	}
	t.Setenv("PATH", dir)

	got, err := installedVmlitePath("")
	if err != nil {
		t.Fatalf("installedVmlitePath: %v", err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

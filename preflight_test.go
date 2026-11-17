package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	tests := [][]string{
		{"a.json", "b.json"},
		{"--bogus"},
	}
	for _, args := range tests {
		if err := run(args); !errors.Is(err, errUsage) {
			t.Fatalf("run(%q) = %v, want errUsage", args, err)
		}
	}
}

func TestRunMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	err := run([]string{path})
	if err == nil {
		t.Fatal("run succeeded, want error")
	}
	if !strings.Contains(err.Error(), "open config") {
		t.Fatalf("error = %q, want open config", err)
	}
}

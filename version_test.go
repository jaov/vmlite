package main

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	old := version
	version = "v1.2.3"
	t.Cleanup(func() { version = old })

	out, err := captureCommandOutput(t, func() error { return runVersion(nil) })
	if err != nil {
		t.Fatalf("runVersion: %v", err)
	}
	if strings.TrimSpace(out) != "vmlite v1.2.3" {
		t.Fatalf("version output = %q", out)
	}

	out, err = captureCommandOutput(t, func() error { return dispatch([]string{"--version"}) })
	if err != nil {
		t.Fatalf("dispatch --version: %v", err)
	}
	if !strings.Contains(out, "vmlite v1.2.3") {
		t.Fatalf("--version output = %q", out)
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	if err := runVersion([]string{"extra"}); err != errUsage {
		t.Fatalf("runVersion = %v, want errUsage", err)
	}
}

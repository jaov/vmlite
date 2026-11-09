package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakePath(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, script := range scripts {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir)
}

func requireKVM(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("/dev/kvm is not available")
	}
}

func TestCheckHostMissingVirtInstall(t *testing.T) {
	fakePath(t, map[string]string{
		"virsh": "exit 0\n",
	})

	err := CheckHost()
	if err == nil {
		t.Fatal("CheckHost succeeded, want error")
	}
	if !strings.Contains(err.Error(), "virt-install") {
		t.Fatalf("error = %q, want virt-install", err)
	}
}

func TestCheckHostMissingVirsh(t *testing.T) {
	fakePath(t, map[string]string{
		"virt-install": "exit 0\n",
	})

	err := CheckHost()
	if err == nil {
		t.Fatal("CheckHost succeeded, want error")
	}
	if !strings.Contains(err.Error(), "virsh") {
		t.Fatalf("error = %q, want virsh", err)
	}
}

func TestCheckHostOK(t *testing.T) {
	requireKVM(t)
	fakePath(t, map[string]string{
		"virt-install": "exit 0\n",
		"virsh":        "exit 0\n",
	})

	if err := CheckHost(); err != nil {
		t.Fatalf("CheckHost: %v", err)
	}
}

func TestDaemonReachable(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   bool
	}{
		{
			name:   "success",
			script: "exit 0\n",
			want:   true,
		},
		{
			name:   "failure",
			script: "exit 1\n",
			want:   false,
		},
		{
			name:   "selects by uri",
			script: "case \"$*\" in *qemu:///system*) exit 0 ;; *) exit 1 ;; esac\n",
			want:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakePath(t, map[string]string{"virsh": tc.script})
			if got := daemonReachable(systemURI); got != tc.want {
				t.Fatalf("daemonReachable = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunPreflightChecksPicksSystem(t *testing.T) {
	requireKVM(t)
	fakePath(t, map[string]string{
		"virt-install": "exit 0\n",
		"virsh":        "exit 0\n",
	})

	got, err := RunPreflightChecks()
	if err != nil {
		t.Fatalf("RunPreflightChecks: %v", err)
	}
	if got != systemURI {
		t.Fatalf("URI = %q, want %q", got, systemURI)
	}
}

func TestRunPreflightChecksFallsBackToSession(t *testing.T) {
	requireKVM(t)
	fakePath(t, map[string]string{
		"virt-install": "exit 0\n",
		"virsh":        "case \"$*\" in *qemu:///system*) exit 1 ;; *) exit 0 ;; esac\n",
	})

	got, err := RunPreflightChecks()
	if err != nil {
		t.Fatalf("RunPreflightChecks: %v", err)
	}
	if got != sessionURI {
		t.Fatalf("URI = %q, want %q", got, sessionURI)
	}
}

func TestRunPreflightChecksNoDaemon(t *testing.T) {
	requireKVM(t)
	fakePath(t, map[string]string{
		"virt-install": "exit 0\n",
		"virsh":        "exit 1\n",
	})

	_, err := RunPreflightChecks()
	if err == nil {
		t.Fatal("RunPreflightChecks succeeded, want error")
	}
	if !strings.Contains(err.Error(), "no accessible libvirt daemon") {
		t.Fatalf("error = %q, want no accessible daemon", err)
	}
}

func TestValidateConnection(t *testing.T) {
	requireKVM(t)

	t.Run("reachable", func(t *testing.T) {
		fakePath(t, map[string]string{
			"virt-install": "exit 0\n",
			"virsh":        "exit 0\n",
		})
		if err := ValidateConnection(systemURI); err != nil {
			t.Fatalf("ValidateConnection: %v", err)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		fakePath(t, map[string]string{
			"virt-install": "exit 0\n",
			"virsh":        "exit 1\n",
		})
		err := ValidateConnection(systemURI)
		if err == nil {
			t.Fatal("ValidateConnection succeeded, want error")
		}
		if !strings.Contains(err.Error(), "cannot connect") {
			t.Fatalf("error = %q, want cannot connect", err)
		}
	})
}

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

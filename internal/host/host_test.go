package host

import (
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

func TestCheckMissingVirtInstall(t *testing.T) {
	fakePath(t, map[string]string{"virsh": "exit 0\n"})
	err := Check()
	if err == nil || !strings.Contains(err.Error(), "virt-install") {
		t.Fatalf("Check = %v, want virt-install", err)
	}
}

func TestCheckMissingVirsh(t *testing.T) {
	fakePath(t, map[string]string{"virt-install": "exit 0\n"})
	err := Check()
	if err == nil || !strings.Contains(err.Error(), "virsh") {
		t.Fatalf("Check = %v, want virsh", err)
	}
}

func TestCheckOK(t *testing.T) {
	requireKVM(t)
	fakePath(t, map[string]string{"virt-install": "exit 0\n", "virsh": "exit 0\n"})
	if err := Check(); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestReachable(t *testing.T) {
	cases := []struct {
		name   string
		script string
		want   bool
	}{
		{"success", "exit 0\n", true},
		{"failure", "exit 1\n", false},
		{"selects by uri", "case \"$*\" in *qemu:///system*) exit 0 ;; *) exit 1 ;; esac\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakePath(t, map[string]string{"virsh": tc.script})
			if got := reachable(SystemURI); got != tc.want {
				t.Fatalf("reachable = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPreflightPicksSystem(t *testing.T) {
	requireKVM(t)
	fakePath(t, map[string]string{"virt-install": "exit 0\n", "virsh": "exit 0\n"})
	got, err := Preflight()
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if got != SystemURI {
		t.Fatalf("URI = %q, want %q", got, SystemURI)
	}
}

func TestPreflightFallsBackToSession(t *testing.T) {
	requireKVM(t)
	fakePath(t, map[string]string{
		"virt-install": "exit 0\n",
		"virsh":        "case \"$*\" in *qemu:///system*) exit 1 ;; *) exit 0 ;; esac\n",
	})
	got, err := Preflight()
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if got != SessionURI {
		t.Fatalf("URI = %q, want %q", got, SessionURI)
	}
}

func TestPreflightNoDaemon(t *testing.T) {
	requireKVM(t)
	fakePath(t, map[string]string{"virt-install": "exit 0\n", "virsh": "exit 1\n"})
	_, err := Preflight()
	if err == nil || !strings.Contains(err.Error(), "no accessible libvirt daemon") {
		t.Fatalf("Preflight = %v", err)
	}
}

func TestValidateConnection(t *testing.T) {
	requireKVM(t)
	t.Run("reachable", func(t *testing.T) {
		fakePath(t, map[string]string{"virt-install": "exit 0\n", "virsh": "exit 0\n"})
		if err := ValidateConnection(SystemURI); err != nil {
			t.Fatalf("ValidateConnection: %v", err)
		}
	})
	t.Run("unreachable", func(t *testing.T) {
		fakePath(t, map[string]string{"virt-install": "exit 0\n", "virsh": "exit 1\n"})
		err := ValidateConnection(SystemURI)
		if err == nil || !strings.Contains(err.Error(), "cannot connect") {
			t.Fatalf("ValidateConnection = %v", err)
		}
	})
}

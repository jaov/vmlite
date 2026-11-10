package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runInstallDryRun(t *testing.T, osRelease string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	releasePath := filepath.Join(dir, "os-release")
	if err := os.WriteFile(releasePath, []byte(osRelease), 0o644); err != nil {
		t.Fatalf("write os-release: %v", err)
	}
	cmd := exec.Command("./install.sh", "--dry-run", "--binary", "/bin/true")
	cmd.Env = append(os.Environ(), "VMLITE_OS_RELEASE="+releasePath)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallScriptDistroPackages(t *testing.T) {
	tests := []struct {
		name    string
		release string
		want    string
	}{
		{"ubuntu", "ID=ubuntu\nID_LIKE=debian\n", "apt-get"},
		{"debian", "ID=debian\n", "apt-get"},
		{"fedora", "ID=fedora\n", "dnf"},
		{"rhel", "ID=rhel\nID_LIKE=\"fedora\"\n", "dnf"},
		{"arch", "ID=arch\n", "pacman"},
		{"opensuse", "ID=opensuse-leap\nID_LIKE=\"suse\"\n", "zypper"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runInstallDryRun(t, tc.release)
			if err != nil {
				t.Fatalf("install.sh failed: %v\n%s", err, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, out)
			}
			if !strings.Contains(out, "install -m 0755 /bin/true") {
				t.Fatalf("output missing binary install:\n%s", out)
			}
		})
	}
}

func TestInstallScriptRejectsFreeBSD(t *testing.T) {
	out, err := runInstallDryRun(t, "ID=freebsd\n")
	if err == nil {
		t.Fatalf("install.sh succeeded for FreeBSD:\n%s", out)
	}
	if !strings.Contains(out, "FreeBSD is not supported") {
		t.Fatalf("output did not explain the FreeBSD limitation:\n%s", out)
	}
}

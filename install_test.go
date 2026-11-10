package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptBootstrap(t *testing.T) {
	dir := t.TempDir()
	releasePath := filepath.Join(dir, "os-release")
	if err := os.WriteFile(releasePath, []byte("ID=ubuntu\nID_LIKE=debian\n"), 0o644); err != nil {
		t.Fatalf("write os-release: %v", err)
	}

	cmd := exec.Command("./install.sh", "--dry-run", "--binary", "/bin/true")
	cmd.Env = append(os.Environ(), "VMLITE_OS_RELEASE="+releasePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh bootstrap failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "apt-get") {
		t.Fatalf("bootstrap did not reach the package plan:\n%s", out)
	}
	if !strings.Contains(string(out), "install /bin/true") {
		t.Fatalf("bootstrap did not reach binary installation:\n%s", out)
	}
}

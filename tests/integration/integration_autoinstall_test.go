//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runVmliteLong(t *testing.T, args []string, timeout time.Duration) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, vmliteBin, args...)
	cmd.Env = envWith(nil)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("vmlite timed out after %s\n%s", timeout, out)
	}
	return string(out), err
}

func TestUbuntuAutoinstall(t *testing.T) {
	if os.Getenv("VMLITE_TEST_UBUNTU_AUTOINSTALL") != "1" {
		t.Skip("set VMLITE_TEST_UBUNTU_AUTOINSTALL=1 to run the Ubuntu autoinstall test")
	}
	iso := os.Getenv("VMLITE_TEST_UBUNTU_ISO")
	if iso == "" {
		t.Skip("set VMLITE_TEST_UBUNTU_ISO to a Ubuntu live-server ISO")
	}
	if _, err := os.Stat(iso); err != nil {
		t.Skipf("VMLITE_TEST_UBUNTU_ISO is not accessible: %v", err)
	}
	requireIntegration(t)
	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt unavailable: %v", err)
	}

	workDir, err := os.MkdirTemp("/tmp", "vmlite-auto-")
	if err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	if err := os.Chmod(workDir, 0o755); err != nil {
		t.Fatalf("chmod work dir: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			t.Logf("VMLITE_TEST_KEEP=1: leaving autoinstall work dir %s", workDir)
			return
		}
		os.RemoveAll(workDir)
	})
	if free := freeBytes(t, workDir); free < 4<<30 {
		t.Skipf("not enough tmpfs space in %s: %d bytes available", workDir, free)
	}

	name := uniqueName()
	for _, pool := range []string{filepath.Base(filepath.Dir(iso)), name, filepath.Base(workDir)} {
		if _, err := virsh(t, systemURI, "pool-info", pool); err == nil {
			continue
		}
		pool := pool
		t.Cleanup(func() {
			if os.Getenv("VMLITE_TEST_KEEP") == "1" {
				return
			}
			_, _ = virsh(t, systemURI, "pool-destroy", pool)
			_, _ = virsh(t, systemURI, "pool-undefine", pool)
		})
	}
	registerCleanup(t, systemURI, name)
	cfg := Config{
		Name:     name,
		Memory:   2048,
		CPUs:     2,
		Firmware: firmwareBIOS,
		Source: Source{
			Type:      sourceISO,
			Path:      iso,
			Installer: &Installer{Type: "ubuntu_autoinstall"},
		},
		Disk:     Disk{Size: 5, Path: filepath.Join(workDir, "disk.qcow2"), Format: diskFormatQCOW2},
		Network:  Network{Mode: networkModeDefault},
		Graphics: Graphics{Type: graphicsNone},
		Provisioners: []Provisioner{
			{Type: "shell", Inline: "echo autoinstall-ok > /var/tmp/vmlite-autoinstall-ok\n", Run: "once"},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	configPath := filepath.Join(workDir, "vm.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	stateDir := filepath.Join(workDir, ".vmlite")

	start := time.Now()
	out, err := runVmliteLong(t, []string{
		"up", "--connect", systemURI, "--state-dir", stateDir, configPath,
	}, 5*time.Minute)
	if err != nil {
		t.Fatalf("up failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Installer") || !strings.Contains(out, "vmlite provision") {
		t.Fatalf("up did not report an installer start:\n%s", out)
	}
	t.Logf("installer started in %s", time.Since(start))

	out, err = runVmliteLong(t, []string{
		"provision", "--state-dir", stateDir, configPath,
	}, 45*time.Minute)
	if err != nil {
		t.Fatalf("provision failed: %v\n%s", err, out)
	}
	t.Logf("autoinstall and provisioning completed in %s", time.Since(start))

	st, err := loadState(filepath.Join(stateDir, name+".json"))
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st == nil {
		t.Fatal("state is missing")
	}
	if st.InstallPhase != installPhaseInstalled {
		t.Fatalf("install phase = %q, want %q", st.InstallPhase, installPhaseInstalled)
	}
	if st.IP == "" || st.SSHKeyPath == "" {
		t.Fatalf("state missing IP/key: %+v", st)
	}

	got, err := sshGuest(t, "vmlite", st.IP, st.SSHKeyPath, "cat /var/tmp/vmlite-autoinstall-ok", 30*time.Second)
	if err != nil {
		t.Fatalf("SSH provisioner check failed: %v\n%s", err, got)
	}
	if strings.TrimSpace(got) != "autoinstall-ok" {
		t.Fatalf("provisioner output = %q", got)
	}

	statusOut, err := runVmliteLong(t, []string{
		"status", "--state-dir", stateDir, configPath,
	}, time.Minute)
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "Install:   installed") || !strings.Contains(statusOut, st.IP) {
		t.Fatalf("status did not report installed state and IP:\n%s", statusOut)
	}
}

//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDebianPreseed(t *testing.T) {
	if os.Getenv("VMLITE_TEST_DEBIAN_AUTOINSTALL") != "1" {
		t.Skip("set VMLITE_TEST_DEBIAN_AUTOINSTALL=1 to run the Debian preseed test")
	}
	iso := os.Getenv("VMLITE_TEST_DEBIAN_ISO")
	if iso == "" {
		t.Skip("set VMLITE_TEST_DEBIAN_ISO to a Debian netinst ISO")
	}
	if _, err := os.Stat(iso); err != nil {
		t.Skipf("VMLITE_TEST_DEBIAN_ISO is not accessible: %v", err)
	}
	requireIntegration(t)
	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt unavailable: %v", err)
	}

	workDir, err := os.MkdirTemp("/tmp", "vmlite-debian-")
	if err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	if err := os.Chmod(workDir, 0o755); err != nil {
		t.Fatalf("chmod work dir: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			t.Logf("VMLITE_TEST_KEEP=1: leaving Debian work dir %s", workDir)
			return
		}
		os.RemoveAll(workDir)
	})
	if free := freeBytes(t, workDir); free < 3<<30 {
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
			Installer: &Installer{Type: "debian_preseed"},
		},
		Disk:     Disk{Size: 5, Path: filepath.Join(workDir, "disk.qcow2"), Format: diskFormatQCOW2},
		Network:  Network{Mode: networkModeDefault},
		Graphics: Graphics{Type: graphicsNone},
		Provisioners: []Provisioner{
			{Type: "shell", Inline: "echo preseed-ok > /var/tmp/vmlite-debian-ok\n", Run: "once"},
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
	t.Logf("preseed installer started in %s", time.Since(start))

	out, err = runVmliteLong(t, []string{
		"provision", "--state-dir", stateDir, configPath,
	}, 45*time.Minute)
	if err != nil {
		t.Fatalf("provision failed: %v\n%s", err, out)
	}
	t.Logf("preseed and provisioning completed in %s", time.Since(start))

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

	got, err := sshGuest(t, "vmlite", st.IP, st.SSHKeyPath, "cat /var/tmp/vmlite-debian-ok", 30*time.Second)
	if err != nil {
		t.Fatalf("SSH provisioner check failed: %v\n%s", err, got)
	}
	if strings.TrimSpace(got) != "preseed-ok" {
		t.Fatalf("provisioner output = %q", got)
	}
}

//go:build integration

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func TestWindowsAutoinstall(t *testing.T) {
	if os.Getenv("VMLITE_TEST_WINDOWS_INSTALL") != "1" {
		t.Skip("set VMLITE_TEST_WINDOWS_INSTALL=1 to run the Windows autounattend test")
	}
	iso := os.Getenv("VMLITE_TEST_WINDOWS_ISO")
	unattend := os.Getenv("VMLITE_TEST_WINDOWS_UNATTEND")
	if iso == "" || unattend == "" {
		t.Skip("set VMLITE_TEST_WINDOWS_ISO and VMLITE_TEST_WINDOWS_UNATTEND")
	}
	if _, err := os.Stat(iso); err != nil {
		t.Skipf("VMLITE_TEST_WINDOWS_ISO is not accessible: %v", err)
	}
	if _, err := os.Stat(unattend); err != nil {
		t.Skipf("VMLITE_TEST_WINDOWS_UNATTEND is not accessible: %v", err)
	}
	requireIntegration(t)
	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt unavailable: %v", err)
	}

	baseDir := os.Getenv("VMLITE_TEST_WORKDIR")
	if baseDir == "" {
		baseDir = "/tmp"
	}
	workDir, err := os.MkdirTemp(baseDir, "vmlite-windows-")
	if err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	if err := os.Chmod(workDir, 0o755); err != nil {
		t.Fatalf("chmod work dir: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			t.Logf("VMLITE_TEST_KEEP=1: leaving Windows work dir %s", workDir)
			return
		}
		os.RemoveAll(workDir)
	})
	if free := freeBytes(t, workDir); free < 40<<30 {
		t.Skipf("not enough space in %s: %d bytes available, need about 40 GiB", workDir, free)
	}

	isoPath, err := filepath.Abs(iso)
	if err != nil {
		t.Fatalf("resolve Windows ISO: %v", err)
	}
	unattendPath, err := filepath.Abs(unattend)
	if err != nil {
		t.Fatalf("resolve autounattend: %v", err)
	}

	name := uniqueName()
	pool := filepath.Base(workDir)
	if _, err := virsh(t, systemURI, "pool-info", pool); err != nil {
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
		Name:       name,
		Memory:     4096,
		CPUs:       2,
		OSVariant:  envDefault("VMLITE_TEST_WINDOWS_OS_VARIANT", "win11"),
		Firmware:   firmwareUEFI,
		SecureBoot: true,
		TPM:        &TPM{Version: tpmVersion20},
		Source: Source{
			Type: sourceISO,
			Path: isoPath,
			Installer: &Installer{
				Type:   "windows_autounattend",
				Config: unattendPath,
			},
		},
		Disk:     Disk{Size: 64, Path: filepath.Join(workDir, "disk.qcow2"), Format: diskFormatQCOW2, Bus: diskBusSATA},
		Network:  Network{Mode: networkModeDefault},
		Graphics: Graphics{Type: graphicsNone},
		Provisioners: []Provisioner{
			{Type: "powershell", Inline: "Set-Content C:\\vmlite-windows-ok.txt 'windows-ok'\n", Run: "once"},
		},
	}
	if os.Getenv("VMLITE_TEST_WINDOWS_LEGACY") == "1" {
		cfg.Firmware = firmwareBIOS
		cfg.SecureBoot = false
		cfg.TPM = nil
		cfg.OSVariant = envDefault("VMLITE_TEST_WINDOWS_OS_VARIANT", "win10")
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
	t.Logf("Windows installer started in %s", time.Since(start))

	out, err = runVmliteLong(t, []string{
		"provision", "--state-dir", stateDir, configPath,
	}, 90*time.Minute)
	if err != nil {
		t.Fatalf("provision failed: %v\n%s", err, out)
	}
	t.Logf("Windows install and provisioning completed in %s", time.Since(start))

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
	if st.InstallerCompletion != "ssh" {
		t.Fatalf("installer completion = %q", st.InstallerCompletion)
	}
	if st.IP == "" || st.SSHKeyPath == "" {
		t.Fatalf("state missing IP/key: %+v", st)
	}

	got, err := sshGuest(t, "Administrator", st.IP, st.SSHKeyPath, `type C:\vmlite-windows-ok.txt`, 60*time.Second)
	if err != nil {
		t.Fatalf("Windows SSH provisioner check failed: %v\n%s", err, got)
	}
	if !strings.Contains(got, "windows-ok") {
		t.Fatalf("provisioner output = %q", got)
	}
}

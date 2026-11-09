package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func baseTestConfig() Config {
	return Config{
		Name:      "demo",
		Memory:    1024,
		CPUs:      2,
		DiskSize:  5,
		OSVariant: "debian12",
	}
}

func TestBuildVirtInstallArgsSystem(t *testing.T) {
	cfg := baseTestConfig()

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	want := []string{
		"--connect", systemURI,
		"--name", "demo",
		"--memory", "1024",
		"--vcpus", "2",
		"--disk", "size=5",
		"--os-variant", "debian12",
		"--graphics", "none",
		"--install", "no_install=yes",
		"--noautoconsole",
		"--network", "network=default",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args =\n%q\nwant\n%q", got, want)
	}
}

func TestBuildVirtInstallArgsSession(t *testing.T) {
	cfg := baseTestConfig()

	got, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	want := []string{
		"--connect", sessionURI,
		"--name", "demo",
		"--memory", "1024",
		"--vcpus", "2",
		"--disk", "size=5",
		"--os-variant", "debian12",
		"--graphics", "none",
		"--install", "no_install=yes",
		"--noautoconsole",
		"--network", "user",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args =\n%q\nwant\n%q", got, want)
	}
}

func TestBuildVirtInstallArgsInstallURL(t *testing.T) {
	cfg := baseTestConfig()
	cfg.InstallURL = "https://example.invalid/debian/"

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	want := []string{
		"--connect", systemURI,
		"--name", "demo",
		"--memory", "1024",
		"--vcpus", "2",
		"--disk", "size=5",
		"--os-variant", "debian12",
		"--graphics", "none",
		"--location", "https://example.invalid/debian/",
		"--extra-args", "console=ttyS0,115200n8",
		"--network", "network=default",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args =\n%q\nwant\n%q", got, want)
	}
}

func TestBuildVirtInstallArgsImage(t *testing.T) {
	dir := t.TempDir()
	cfg := baseTestConfig()
	cfg.Image = filepath.Join(dir, "base.qcow2")
	cfg.DiskPath = filepath.Join(dir, "overlay.qcow2")
	cfg.DiskSize = 10

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	want := []string{
		"--connect", systemURI,
		"--name", "demo",
		"--memory", "1024",
		"--vcpus", "2",
		"--os-variant", "debian12",
		"--graphics", "none",
		"--import",
		"--disk", "path=" + cfg.DiskPath + ",format=qcow2",
		"--noautoconsole",
		"--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart",
		"--network", "network=default",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args =\n%q\nwant\n%q", got, want)
	}
}

func TestBuildVirtInstallArgsImageCloudInit(t *testing.T) {
	dir := t.TempDir()
	userData := filepath.Join(dir, "user-data")
	if err := os.WriteFile(userData, []byte("#cloud-config\n"), 0o644); err != nil {
		t.Fatalf("write user-data: %v", err)
	}

	cfg := baseTestConfig()
	cfg.Image = filepath.Join(dir, "base.qcow2")
	cfg.DiskPath = filepath.Join(dir, "overlay.qcow2")
	cfg.CloudInit = userData
	cfg.DiskSize = 10

	got, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	seed := filepath.Join(dir, "demo-seed.iso")
	if !containsAll(got,
		"--import",
		"path="+seed+",device=cdrom,readonly=on",
		"--sysinfo", "type=smbios,system.serial=ds=nocloud",
		"--network", "user",
	) {
		t.Fatalf("args missing seed/sysinfo options: %q", got)
	}
	if containsAll(got, "--cloud-init") {
		t.Fatalf("image mode should use its own seed, not --cloud-init: %q", got)
	}
	if containsAll(got, "size=10") {
		t.Fatalf("image mode should not use --disk size=: %q", got)
	}
}

func containsAll(haystack []string, needles ...string) bool {
	index := make(map[string]bool, len(haystack))
	for _, s := range haystack {
		index[s] = true
	}
	for _, n := range needles {
		if !index[n] {
			return false
		}
	}
	return true
}

func TestBuildVirtInstallArgsMounts(t *testing.T) {
	dir := t.TempDir()
	cfg := baseTestConfig()
	cfg.Mounts = []Mount{{Source: dir, Target: "shared_data"}}

	got, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	absSource, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	wantTail := []string{
		"--filesystem",
		"type=mount,source=" + absSource + ",target=shared_data,accessmode=mapped",
	}
	if !reflect.DeepEqual(got[len(got)-len(wantTail):], wantTail) {
		t.Fatalf("mount args = %q, want %q", got[len(got)-len(wantTail):], wantTail)
	}
}

func TestBuildVirtInstallArgsMissingMount(t *testing.T) {
	cfg := baseTestConfig()
	cfg.Mounts = []Mount{{Source: filepath.Join(t.TempDir(), "missing"), Target: "data"}}

	_, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err == nil {
		t.Fatal("buildVirtInstallArgs succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("error = %q, want not accessible", err)
	}
}

func TestBuildVirtInstallArgsUnsupportedURI(t *testing.T) {
	cfg := baseTestConfig()

	_, err := buildVirtInstallArgs(&cfg, "qemu:///bogus")
	if err == nil {
		t.Fatal("buildVirtInstallArgs succeeded, want error")
	}
	if !strings.Contains(err.Error(), "unsupported libvirt URI") {
		t.Fatalf("error = %q, want unsupported URI", err)
	}
}

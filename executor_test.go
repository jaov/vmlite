package main

import (
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

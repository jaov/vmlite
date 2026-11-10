package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeSeedISOBuilder(t *testing.T) (argsLog string) {
	t.Helper()
	binDir := t.TempDir()
	argsLog = filepath.Join(binDir, "args.log")
	script := fmt.Sprintf(`#!/bin/sh
out=""
prev=""
last=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out="$a"; fi
  prev="$a"
  last="$a"
done
printf '%%s\n' "$@" > %q
for f in "$last"/*; do
  printf 'FILE=%%s\n' "${f##*/}" >> %q
  /bin/cat "$f" >> %q
done
: > "$out"
exit 0
`, argsLog, argsLog, argsLog)
	if err := os.WriteFile(filepath.Join(binDir, "xorrisofs"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake xorrisofs: %v", err)
	}
	t.Setenv("PATH", binDir)
	return argsLog
}

func TestPrepareInstallerSeed(t *testing.T) {
	argsLog := fakeSeedISOBuilder(t)
	cfg := &Config{
		Name:         "auto",
		Source:       Source{Type: sourceISO, Path: "/tmp/ubuntu.iso", Installer: &Installer{Type: "ubuntu_autoinstall"}},
		assetDir:     t.TempDir(),
		sshPublicKey: "ssh-ed25519 AAAA test",
	}
	path, err := prepareInstallerSeed(cfg)
	if err != nil {
		t.Fatalf("prepareInstallerSeed: %v", err)
	}
	want := filepath.Join(cfg.assetDir, "auto-installer-seed.iso")
	if path != want {
		t.Fatalf("seed path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("seed not created: %v", err)
	}

	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read builder log: %v", err)
	}
	log := string(data)
	for _, want := range []string{"-V\ncidata", "FILE=user-data", "FILE=meta-data", "autoinstall:", "ssh-ed25519 AAAA test"} {
		if !strings.Contains(log, want) {
			t.Errorf("builder log missing %q:\n%s", want, log)
		}
	}
}

func TestNewStateInstaller(t *testing.T) {
	layout := projectLayout{
		ConfigPath: "/tmp/vm.json",
		StateFile:  "/tmp/.vmlite/auto.json",
		AssetDir:   "/tmp/.vmlite/auto",
	}
	cfg := &Config{
		Name:         "auto",
		Source:       Source{Type: sourceISO, Path: "/tmp/ubuntu.iso", Installer: &Installer{Type: "ubuntu_autoinstall"}},
		assetDir:     layout.AssetDir,
		sshPublicKey: "ssh-ed25519 AAAA test",
	}
	applyProjectDefaults(cfg, layout)

	st, err := newState(cfg, layout, systemURI, "hash", "uuid")
	if err != nil {
		t.Fatalf("newState: %v", err)
	}
	if st.InstallerType != "ubuntu_autoinstall" || st.InstallPhase != installPhaseInstalling {
		t.Fatalf("installer state = %+v", st)
	}
	wantSeed := filepath.Join(layout.AssetDir, "auto-installer-seed.iso")
	if st.SeedPath != wantSeed {
		t.Fatalf("seed path = %q, want %q", st.SeedPath, wantSeed)
	}
}

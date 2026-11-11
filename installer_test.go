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

func TestNewStateWindowsInstaller(t *testing.T) {
	layout := projectLayout{
		ConfigPath: "/tmp/vm.json",
		StateFile:  "/tmp/.vmlite/win.json",
		AssetDir:   "/tmp/.vmlite/win",
	}
	cfg := &Config{
		Name: "win",
		Source: Source{
			Type:      sourceISO,
			Path:      "/tmp/win.iso",
			Installer: &Installer{Type: "windows_autounattend", Inline: "<unattend/>"},
		},
		assetDir: layout.AssetDir,
	}
	applyProjectDefaults(cfg, layout)
	st, err := newState(cfg, layout, systemURI, "hash", "uuid")
	if err != nil {
		t.Fatalf("newState: %v", err)
	}
	if st.InstallerCompletion != "ssh" {
		t.Fatalf("completion = %q", st.InstallerCompletion)
	}
	if st.SeedPath != filepath.Join(layout.AssetDir, "win-installer-seed.iso") {
		t.Fatalf("seed = %q", st.SeedPath)
	}
}

func TestPrepareInstallerISO(t *testing.T) {
	binDir := t.TempDir()
	argsLog := filepath.Join(binDir, "xorriso.log")
	script := "#!/bin/sh\n" +
		"out=\"\"\n" +
		"prev=\"\"\n" +
		"for a in \"$@\"; do\n" +
		"  printf '%s\\n' \"$a\" >> " + argsLog + "\n" +
		"  if [ \"$prev\" = \"-outdev\" ]; then out=\"$a\"; fi\n" +
		"  prev=\"$a\"\n" +
		"done\n" +
		": > \"$out\"\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "xorriso"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake xorriso: %v", err)
	}
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	source := filepath.Join(dir, "debian.iso")
	if err := os.WriteFile(source, []byte("iso"), 0o644); err != nil {
		t.Fatalf("write source ISO: %v", err)
	}
	cfg := &Config{
		Name:         "debian",
		Source:       Source{Type: sourceISO, Path: source, Installer: &Installer{Type: "debian_preseed"}},
		assetDir:     t.TempDir(),
		sshPublicKey: "ssh-ed25519 AAAA test",
	}
	spec, err := cfg.installerSpec()
	if err != nil {
		t.Fatalf("installerSpec: %v", err)
	}
	path, err := prepareInstallerISO(cfg, spec)
	if err != nil {
		t.Fatalf("prepareInstallerISO: %v", err)
	}
	if path != filepath.Join(cfg.assetDir, "debian-installer.iso") {
		t.Fatalf("path = %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("remastered ISO missing: %v", err)
	}
	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read xorriso log: %v", err)
	}
	log := string(data)
	for _, want := range []string{"-indev", source, "-outdev", ".installer-", "-boot_image", "any", "replay", "/preseed.cfg"} {
		if !strings.Contains(log, want) {
			t.Errorf("xorriso log missing %q:\n%s", want, log)
		}
	}
}

func TestPrepareInstallerKernel(t *testing.T) {
	binDir := t.TempDir()
	argsLog := filepath.Join(binDir, "xorriso.log")
	script := "#!/bin/sh\n" +
		"out=\"\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  printf '%s\\n' \"$1\" >> " + argsLog + "\n" +
		"  case \"$1\" in\n" +
		"    -outdev) out=\"$2\"; shift 2 ;;\n" +
		"    -extract) printf '%s\\n%s\\n' \"$2\" \"$3\" >> " + argsLog + "; printf 'arch-kernel-%s' \"$(basename \"$2\")\" > \"$3\"; shift 3 ;;\n" +
		"    *) shift ;;\n" +
		"  esac\n" +
		"done\n" +
		"if [ -n \"$out\" ]; then : > \"$out\"; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "xorriso"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake xorriso: %v", err)
	}
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	source := filepath.Join(dir, "arch.iso")
	if err := os.WriteFile(source, []byte("iso"), 0o644); err != nil {
		t.Fatalf("write source ISO: %v", err)
	}
	cfg := &Config{
		Name:         "arch",
		Source:       Source{Type: sourceISO, Path: source, Installer: &Installer{Type: "arch_script"}},
		Disk:         Disk{Size: 8, Path: filepath.Join(dir, "disk.qcow2"), Format: diskFormatQCOW2},
		assetDir:     filepath.Join(dir, ".vmlite", "arch"),
		sshPublicKey: "ssh-ed25519 AAAA test",
	}
	spec, err := cfg.installerSpec()
	if err != nil {
		t.Fatalf("installerSpec: %v", err)
	}
	kernel, initrd, err := prepareInstallerKernel(cfg, spec)
	if err != nil {
		t.Fatalf("prepareInstallerKernel: %v", err)
	}
	for _, path := range []string{kernel, initrd} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read extracted file %q: %v", path, err)
		}
		if !strings.Contains(string(data), "arch-kernel-") {
			t.Fatalf("unexpected extracted data in %q: %q", path, data)
		}
	}
	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read xorriso log: %v", err)
	}
	log := string(data)
	for _, want := range []string{
		"-osirrox", "on", "-extract", spec.KernelPath, "-extract", spec.InitrdPath,
	} {
		if !strings.Contains(log, want) {
			t.Errorf("xorriso log missing %q:\n%s", want, log)
		}
	}

	args, err := buildSourceArgs(cfg, spec)
	if err != nil {
		t.Fatalf("buildSourceArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--cdrom", "kernel=" + kernel, "initrd=" + initrd, "kernel_args=archisobasedir=arch"} {
		if !strings.Contains(joined, want) {
			t.Errorf("source args missing %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "--location") {
		t.Errorf("kernel boot should not use --location: %s", joined)
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
	if st.InstallerType != "ubuntu_autoinstall" || st.InstallPhase != installPhaseInstalling || st.InstallerCompletion != "shutdown" {
		t.Fatalf("installer state = %+v", st)
	}
	wantSeed := filepath.Join(layout.AssetDir, "auto-installer-seed.iso")
	if st.SeedPath != wantSeed {
		t.Fatalf("seed path = %q, want %q", st.SeedPath, wantSeed)
	}
}

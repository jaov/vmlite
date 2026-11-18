package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	guestinstall "vmlite/internal/installer"
)

func baseTestConfig() Config {
	return Config{
		Name:      "demo",
		Memory:    1024,
		CPUs:      2,
		OSVariant: "debian12",
		Firmware:  firmwareBIOS,
		Source:    Source{Type: sourceNone},
		Disk:      Disk{Size: 5},
		Network:   Network{Mode: networkModeAuto},
		Graphics:  Graphics{Type: graphicsNone},
	}
}

func TestBuildVirtInstallArgsNoneSystem(t *testing.T) {
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
		"--os-variant", "debian12",
		"--disk", "size=5",
		"--install", "no_install=yes",
		"--graphics", "none",
		"--noautoconsole",
		"--network", "network=default",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args =\n%q\nwant\n%q", got, want)
	}
}

func TestBuildVirtInstallArgsNoneSession(t *testing.T) {
	cfg := baseTestConfig()

	got, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	if !containsAll(got, "--network", "user") {
		t.Fatalf("args missing user network: %q", got)
	}
}

func TestBuildVirtInstallArgsCloudImage(t *testing.T) {
	dir := t.TempDir()
	cfg := baseTestConfig()
	cfg.Source = Source{Type: sourceCloudImage, Path: filepath.Join(dir, "base.qcow2")}
	cfg.Disk = Disk{Size: 10, Path: filepath.Join(dir, "overlay.qcow2"), Format: diskFormatQCOW2}

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
		"--import",
		"--disk", "path=" + cfg.Disk.Path + ",format=qcow2",
		"--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart",
		"--graphics", "none",
		"--noautoconsole",
		"--network", "network=default",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args =\n%q\nwant\n%q", got, want)
	}
}

func TestBuildVirtInstallArgsCloudInit(t *testing.T) {
	dir := t.TempDir()
	cfg := baseTestConfig()
	cfg.Source = Source{
		Type:      sourceCloudImage,
		Path:      filepath.Join(dir, "base.qcow2"),
		CloudInit: filepath.Join(dir, "user-data"),
	}
	cfg.Disk = Disk{Size: 10, Path: filepath.Join(dir, "overlay.qcow2")}

	got, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	seed := filepath.Join(dir, "demo-seed.iso")
	if !containsAll(got,
		"--import",
		"path="+seed+",format=raw,readonly=on",
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

func TestBuildVirtInstallArgsInstallTree(t *testing.T) {
	cfg := baseTestConfig()
	cfg.Source = Source{
		Type:      sourceInstallTree,
		URL:       "https://example.invalid/debian/",
		ExtraArgs: "console=ttyS0,115200n8",
	}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	if !containsAll(got,
		"--disk", "size=5",
		"--location", "https://example.invalid/debian/",
		"--extra-args", "console=ttyS0,115200n8",
		"--noautoconsole",
	) {
		t.Fatalf("args missing install-tree options: %q", got)
	}
}

func TestBuildVirtInstallArgsISO(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "install.iso")
	cfg := baseTestConfig()
	cfg.OSVariant = ""
	cfg.Source = Source{Type: sourceISO, Path: iso}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	if !containsAll(got,
		"--os-variant", "detect=on,require=off",
		"--disk", "size=5",
		"--cdrom", iso,
		"--noautoconsole",
	) {
		t.Fatalf("args missing iso options: %q", got)
	}
}

func TestBuildVirtInstallArgsDiskSource(t *testing.T) {
	dir := t.TempDir()
	disk := filepath.Join(dir, "freebsd.qcow2")
	cfg := baseTestConfig()
	cfg.Source = Source{Type: sourceDisk, Path: disk}
	cfg.Disk = Disk{Format: diskFormatQCOW2}

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
		"--import",
		"--disk", "path=" + disk + ",format=qcow2",
		"--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart",
		"--graphics", "none",
		"--noautoconsole",
		"--network", "network=default",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args =\n%q\nwant\n%q", got, want)
	}
}

func TestBuildVirtInstallArgsDiskSourceAuto(t *testing.T) {
	binDir, _ := fakeQEMUImgInfo(t, diskFormatRaw)
	t.Setenv("PATH", binDir)

	disk := filepath.Join(t.TempDir(), "freebsd.raw")
	cfg := baseTestConfig()
	cfg.Source = Source{Type: sourceDisk, Path: disk}
	cfg.Disk = Disk{Format: diskFormatAuto}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--import", "path="+disk+",format=raw") {
		t.Fatalf("args missing detected raw import: %q", got)
	}
}

func TestBuildVirtInstallArgsPXE(t *testing.T) {
	cfg := baseTestConfig()
	cfg.Source = Source{Type: sourcePXE}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--disk", "size=5", "--pxe") {
		t.Fatalf("args missing pxe options: %q", got)
	}
}

func TestBuildVirtInstallArgsPrimaryDiskPath(t *testing.T) {
	dir := t.TempDir()
	cfg := baseTestConfig()
	cfg.Disk = Disk{Size: 5, Path: filepath.Join(dir, "disk.qcow2"), Format: diskFormatQCOW2}

	got, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	wantDisk := "path=" + cfg.Disk.Path + ",size=5,format=qcow2"
	if !containsAll(got, "--disk", wantDisk) {
		t.Fatalf("args missing primary disk: %q", got)
	}
}

func TestBuildVirtInstallArgsUEFI(t *testing.T) {
	cfg := baseTestConfig()
	cfg.Firmware = firmwareUEFI

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--boot", "uefi") {
		t.Fatalf("args missing uefi boot: %q", got)
	}
}

func TestBootArgs(t *testing.T) {
	cases := []struct {
		firmware string
		order    []string
		secure   bool
		want     []string
	}{
		{firmwareBIOS, nil, false, nil},
		{firmwareUEFI, nil, false, []string{"--boot", "uefi"}},
		{firmwareUEFI, nil, true, []string{"--boot", "uefi,loader.secure=on"}},
		{firmwareBIOS, []string{"hd", "cdrom"}, false, []string{"--boot", "hd,cdrom"}},
		{firmwareUEFI, []string{"hd", "cdrom"}, false, []string{"--boot", "uefi,hd,cdrom"}},
		{firmwareUEFI, []string{"hd", "cdrom"}, true, []string{"--boot", "uefi,loader.secure=on,hd,cdrom"}},
	}
	for _, tc := range cases {
		got := bootArgs(tc.firmware, tc.order, tc.secure)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("bootArgs(%q, %v, %v) = %q, want %q", tc.firmware, tc.order, tc.secure, got, tc.want)
		}
	}
}

func TestBuildVirtInstallArgsISOInstaller(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "ubuntu.iso")
	cfg := baseTestConfig()
	cfg.OSVariant = ""
	cfg.Source = Source{
		Type:      sourceISO,
		Path:      iso,
		Installer: &Installer{Type: "ubuntu_autoinstall"},
	}
	cfg.AssetDir = t.TempDir()
	cfg.SSHPublicKey = "ssh-ed25519 AAAA test"

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}

	seedPath := filepath.Join(cfg.AssetDir, "demo-installer-seed.iso")
	if !containsAll(got,
		"--os-variant", "detect=on,require=off",
		"--boot", "hd,cdrom",
		"--check", "disk_size=off",
		"--location", iso,
		"--extra-args", "autoinstall",
		"--disk", "path="+seedPath+",format=raw,readonly=on,device=disk,bus=virtio",
		"--sysinfo", "type=smbios,system.serial=ds=nocloud",
	) {
		t.Fatalf("args missing installer media: %q", got)
	}
	if containsAll(got, "--cdrom") {
		t.Fatalf("installer with kernel args should use --location, not --cdrom: %q", got)
	}
}

func TestBuildVirtInstallArgsDebianPreseed(t *testing.T) {
	iso := filepath.Join(t.TempDir(), "debian.iso")
	cfg := baseTestConfig()
	cfg.OSVariant = ""
	cfg.Source = Source{
		Type:      sourceISO,
		Path:      iso,
		Installer: &Installer{Type: "debian_preseed"},
	}
	cfg.AssetDir = t.TempDir()
	cfg.SSHPublicKey = "ssh-ed25519 AAAA test"

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	remastered := filepath.Join(cfg.AssetDir, "demo-installer.iso")
	if !containsAll(got,
		"--boot", "hd,cdrom",
		"--check", "disk_size=off",
		"--disk", "size=5,bus=virtio",
		"--location", remastered,
		"--extra-args", "auto=true priority=critical preseed/file=/cdrom/preseed.cfg",
	) {
		t.Fatalf("args missing Debian installer media: %q", got)
	}
	if containsAll(got, "--disk", "path="+filepath.Join(cfg.AssetDir, "demo-installer-seed.iso")) {
		t.Fatalf("Debian preseed should be injected, not attached as a seed: %q", got)
	}
}

func TestBuildVirtInstallArgsFedoraKickstart(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "fedora.iso")
	cfg := baseTestConfig()
	cfg.OSVariant = ""
	cfg.Source = Source{
		Type:      sourceISO,
		Path:      iso,
		Installer: &Installer{Type: "fedora_kickstart"},
	}
	cfg.AssetDir = t.TempDir()
	cfg.SSHPublicKey = "ssh-ed25519 AAAA test"

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	seedPath := filepath.Join(cfg.AssetDir, "demo-installer-seed.iso")
	if !containsAll(got,
		"--boot", "hd,cdrom",
		"--check", "disk_size=off",
		"--cdrom", iso,
		"--disk", "path="+seedPath+",format=raw,readonly=on,device=disk,bus=virtio",
	) {
		t.Fatalf("args missing Fedora installer media: %q", got)
	}
	if containsAll(got, "--location") {
		t.Fatalf("Fedora kickstart should boot from --cdrom: %q", got)
	}
}

func TestBuildVirtInstallArgsISOInstallerUEFI(t *testing.T) {
	cfg := baseTestConfig()
	cfg.OSVariant = ""
	cfg.Firmware = firmwareUEFI
	cfg.Source = Source{
		Type:      sourceISO,
		Path:      "/tmp/ubuntu.iso",
		Installer: &Installer{Type: "ubuntu_autoinstall"},
	}
	cfg.AssetDir = t.TempDir()
	cfg.SSHPublicKey = "ssh-ed25519 AAAA test"

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--boot", "uefi,hd,cdrom") {
		t.Fatalf("args missing uefi installer boot order: %q", got)
	}
}

func TestBuildVirtInstallArgsWindowsDevices(t *testing.T) {
	cfg := baseTestConfig()
	cfg.OSVariant = "win11"
	cfg.Firmware = firmwareUEFI
	cfg.SecureBoot = true
	cfg.TPM = &TPM{Version: tpmVersion20}
	cfg.Disk.Bus = diskBusSATA

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got,
		"--boot", "uefi,loader.secure=on",
		"--tpm", "backend.type=emulator,backend.version=2.0",
		"--disk", "size=5,bus=sata",
	) {
		t.Fatalf("args missing Windows device settings: %q", got)
	}
}

func TestBuildVirtInstallArgsDiskSourceBus(t *testing.T) {
	disk := filepath.Join(t.TempDir(), "windows.qcow2")
	cfg := baseTestConfig()
	cfg.Source = Source{Type: sourceDisk, Path: disk}
	cfg.Disk = Disk{Format: diskFormatQCOW2, Bus: diskBusSATA}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--import", "path="+disk+",format=qcow2,bus=sata") {
		t.Fatalf("args missing disk bus: %q", got)
	}
}

func TestBuildVirtInstallArgsWindowsAutoUnattend(t *testing.T) {
	iso := filepath.Join(t.TempDir(), "win11.iso")
	cfg := baseTestConfig()
	cfg.OSVariant = "win11"
	cfg.Firmware = firmwareUEFI
	cfg.SecureBoot = true
	cfg.TPM = &TPM{Version: tpmVersion20}
	cfg.Source = Source{
		Type: sourceISO,
		Path: iso,
		Installer: &Installer{
			Type:   "windows_autounattend",
			Inline: "<unattend/>",
		},
	}
	cfg.AssetDir = t.TempDir()
	cfg.SSHPublicKey = "ssh-ed25519 AAAA test"

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	seedPath := filepath.Join(cfg.AssetDir, "demo-installer-seed.iso")
	if !containsAll(got,
		"--boot", "uefi,loader.secure=on,hd,cdrom",
		"--tpm", "backend.type=emulator,backend.version=2.0",
		"--disk", "size=5,bus=sata",
		"--cdrom", iso,
		"--disk", "path="+seedPath+",format=raw,readonly=on,device=cdrom",
		"--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart",
	) {
		t.Fatalf("args missing Windows installer settings: %q", got)
	}
}

func TestConfigureInstallerLifecycle(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "virsh.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + logPath + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake virsh: %v", err)
	}
	t.Setenv("PATH", binDir)

	cfg := &Config{Name: "win"}
	if err := configureInstallerLifecycle(cfg, systemURI, guestinstall.Spec{Completion: guestinstall.CompletionShutdown}); err != nil {
		t.Fatalf("shutdown lifecycle: %v", err)
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("shutdown installer unexpectedly changed lifecycle")
	}
	if err := configureInstallerLifecycle(cfg, systemURI, guestinstall.Spec{Completion: guestinstall.CompletionSSH}); err != nil {
		t.Fatalf("ssh lifecycle: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read virsh log: %v", err)
	}
	for _, want := range []string{"set-lifecycle-action", "win", "reboot", "restart", "--live", "--config"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("virsh log missing %q:\n%s", want, data)
		}
	}
}

func TestBuildVirtInstallArgsGraphics(t *testing.T) {
	cfg := baseTestConfig()
	cfg.Graphics = Graphics{Type: graphicsVNC, Listen: "127.0.0.1"}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--graphics", "vnc,listen=127.0.0.1") {
		t.Fatalf("args missing vnc graphics: %q", got)
	}

	cfg.Graphics = Graphics{Type: graphicsSPICE}
	got, err = buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--graphics", "spice,listen=127.0.0.1") {
		t.Fatalf("args missing spice graphics: %q", got)
	}
}

func TestBuildVirtInstallArgsAutoconsole(t *testing.T) {
	cfg := baseTestConfig()
	cfg.Autoconsole = true

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if containsAll(got, "--noautoconsole") {
		t.Fatalf("args unexpectedly contain --noautoconsole: %q", got)
	}
}

func TestBuildVirtInstallArgsBridge(t *testing.T) {
	cfg := baseTestConfig()
	cfg.Network = Network{Mode: networkModeBridge, Bridge: "br0", Model: "virtio"}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--network", "bridge=br0,model=virtio") {
		t.Fatalf("args missing bridge network: %q", got)
	}
}

func TestBuildVirtInstallArgsNetworkMAC(t *testing.T) {
	cfg := baseTestConfig()
	cfg.NetworkMAC = "52:54:00:aa:bb:cc"

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--network", "network=default,mac=52:54:00:aa:bb:cc") {
		t.Fatalf("args missing network MAC: %q", got)
	}
}

func TestBuildVirtInstallArgsExtraDisks(t *testing.T) {
	dir := t.TempDir()
	cfg := baseTestConfig()
	cfg.ExtraDisks = []ExtraDisk{
		{Path: filepath.Join(dir, "config.iso"), Device: "cdrom", Format: diskFormatRaw, ReadOnly: true},
	}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	want := "path=" + filepath.Join(dir, "config.iso") + ",device=cdrom,format=raw,readonly=on"
	if !containsAll(got, "--disk", want) {
		t.Fatalf("args missing extra disk: %q", got)
	}
}

func TestBuildVirtInstallArgsExtraDiskAuto(t *testing.T) {
	binDir, _ := fakeQEMUImgInfo(t, diskFormatRaw)
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	diskPath := filepath.Join(dir, "config.iso")
	cfg := baseTestConfig()
	cfg.ExtraDisks = []ExtraDisk{{Path: diskPath, Device: "cdrom", Format: diskFormatAuto}}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	want := "path=" + diskPath + ",device=cdrom,format=raw"
	if !containsAll(got, "--disk", want) {
		t.Fatalf("args missing detected extra disk: %q", got)
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

func TestBuildVirtInstallArgsMountNone(t *testing.T) {
	dir := t.TempDir()
	cfg := baseTestConfig()
	cfg.Mounts = []Mount{{Source: dir, Target: "shared_data", Type: mountTypeNone}}

	got, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if containsAll(got, "--filesystem") {
		t.Fatalf("args unexpectedly contain --filesystem: %q", got)
	}
}

func TestBuildVirtInstallArgsVirtioFSRejected(t *testing.T) {
	cfg := baseTestConfig()
	cfg.Mounts = []Mount{{Source: t.TempDir(), Target: "shared", Type: mountTypeVirtioFS}}

	_, err := buildVirtInstallArgs(&cfg, sessionURI)
	if err == nil {
		t.Fatal("buildVirtInstallArgs succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("error = %q, want not implemented", err)
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

func TestBuildVirtInstallArgsDiskSourceRaw(t *testing.T) {
	disk := filepath.Join(t.TempDir(), "freebsd.raw")
	cfg := baseTestConfig()
	cfg.Source = Source{Type: sourceDisk, Path: disk}
	cfg.Disk = Disk{Format: diskFormatRaw}

	got, err := buildVirtInstallArgs(&cfg, systemURI)
	if err != nil {
		t.Fatalf("buildVirtInstallArgs: %v", err)
	}
	if !containsAll(got, "--import", "path="+disk+",format=raw") {
		t.Fatalf("args missing raw import: %q", got)
	}
}

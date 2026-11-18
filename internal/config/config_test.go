package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vm.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigValid(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "test-vm",
		"memory": 2048,
		"cpus": 2,
		"os_variant": "debian12",
		"firmware": "uefi",
		"source": {
			"type": "cloud_image",
			"path": "/tmp/base.qcow2",
			"cloud_init": "/tmp/user-data"
		},
		"disk": {
			"size": 20,
			"path": "/tmp/overlay.qcow2",
			"format": "qcow2"
		},
		"mounts": [
			{"source": "/tmp/data", "target": "shared_data", "type": "9p"}
		],
		"network": {"mode": "bridge", "bridge": "br0", "model": "virtio"},
		"graphics": {"type": "vnc", "listen": "127.0.0.1"},
		"autoconsole": true,
		"autostart": true,
		"extra_disks": [
			{"path": "/tmp/config.iso", "device": "cdrom", "format": "raw", "readonly": true}
		]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Name != "test-vm" || cfg.Memory != 2048 || cfg.CPUs != 2 {
		t.Errorf("basic fields = %+v", cfg)
	}
	if cfg.OSVariant != "debian12" || cfg.Firmware != "uefi" {
		t.Errorf("os/firmware = %q/%q", cfg.OSVariant, cfg.Firmware)
	}
	if cfg.Source.Type != SourceCloudImage || cfg.Source.Path != "/tmp/base.qcow2" || cfg.Source.CloudInit != "/tmp/user-data" {
		t.Errorf("source = %+v", cfg.Source)
	}
	if cfg.Disk.Size != 20 || cfg.Disk.Path != "/tmp/overlay.qcow2" || cfg.Disk.Format != "qcow2" {
		t.Errorf("disk = %+v", cfg.Disk)
	}
	if len(cfg.Mounts) != 1 || cfg.Mounts[0].Type != "9p" {
		t.Errorf("mounts = %+v", cfg.Mounts)
	}
	if cfg.Network.Mode != "bridge" || cfg.Network.Bridge != "br0" || cfg.Network.Model != "virtio" {
		t.Errorf("network = %+v", cfg.Network)
	}
	if cfg.Graphics.Type != "vnc" || cfg.Graphics.Listen != "127.0.0.1" {
		t.Errorf("graphics = %+v", cfg.Graphics)
	}
	if !cfg.Autoconsole {
		t.Error("Autoconsole = false, want true")
	}
	if !cfg.Autostart {
		t.Error("Autostart = false, want true")
	}
	if len(cfg.ExtraDisks) != 1 || !cfg.ExtraDisks[0].ReadOnly {
		t.Errorf("extra disks = %+v", cfg.ExtraDisks)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	base := func(overrides string) string {
		return `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 5}` + overrides + `}`
	}

	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"missing name", `{"memory": 1024, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 5}}`, "name is required"},
		{"zero memory", `{"name": "vm", "memory": 0, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 5}}`, "memory must be greater than zero"},
		{"negative cpus", `{"name": "vm", "memory": 1024, "cpus": -1, "source": {"type": "none"}, "disk": {"size": 5}}`, "cpus must be greater than zero"},
		{"missing source", `{"name": "vm", "memory": 1024, "cpus": 1, "disk": {"size": 5}}`, "source.type must be"},
		{"invalid source", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "bogus"}, "disk": {"size": 5}}`, "source.type must be"},
		{"none with path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "none", "path": "/tmp/x"}, "disk": {"size": 5}}`, "does not accept path"},
		{"cloud image no path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image"}, "disk": {"size": 5}}`, "requires exactly one"},
		{"cloud image both", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x", "url": "https://example.invalid/x.qcow2"}, "disk": {"size": 5}}`, "requires exactly one"},
		{"cloud image url no sha", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "url": "https://example.invalid/x.qcow2"}, "disk": {"size": 5}}`, "sha256"},
		{"cloud image bad sha", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "url": "https://example.invalid/x.qcow2", "sha256": "nope"}, "disk": {"size": 5}}`, "sha256"},
		{"cloud image cache no url", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x", "cache": "/tmp/cache"}, "disk": {"size": 5}}`, "cache requires"},
		{"cloud image no disk size", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 0}}`, "disk.size"},
		{"install tree no url", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "install_tree"}, "disk": {"size": 5}}`, "source.url is required"},
		{"install tree cloud init", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "install_tree", "url": "https://example.invalid", "cloud_init": "/tmp/u"}, "disk": {"size": 5}}`, "not valid"},
		{"iso no path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso"}, "disk": {"size": 5}}`, "requires exactly one"},
		{"iso url no sha", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso", "url": "https://example.invalid/x.iso"}, "disk": {"size": 5}}`, "sha256"},
		{"iso path and url", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso", "path": "/tmp/x.iso", "url": "https://example.invalid/x.iso", "sha256": "` + strings.Repeat("a", 64) + `"}, "disk": {"size": 5}}`, "exactly one"},
		{"iso cache without url", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso", "path": "/tmp/x.iso", "cache": "/tmp/cache"}, "disk": {"size": 5}}`, "cache requires"},
		{"disk no path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "disk"}, "disk": {"size": 5}}`, "requires exactly one"},
		{"disk url no sha", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "disk", "url": "https://example.invalid/x.qcow2"}, "disk": {"size": 5}}`, "sha256"},
		{"disk with disk path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "disk", "path": "/tmp/x"}, "disk": {"size": 5, "path": "/tmp/y"}}`, "disk.path is not used"},
		{"pxe with path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "pxe", "path": "/tmp/x"}, "disk": {"size": 5}}`, "does not accept path"},
		{"bad firmware", base(`, "firmware": "coreboot"`), "firmware must be"},
		{"bad disk format", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 5, "format": "vmdk"}}`, "disk.format must be"},
		{"mount missing source", base(`, "mounts": [{"target": "data"}]`), "mounts[0].source is required"},
		{"mount missing target", base(`, "mounts": [{"source": "/tmp/data"}]`), "mounts[0].target is required"},
		{"bad mount type", base(`, "mounts": [{"source": "/tmp/data", "target": "data", "type": "nfs"}]`), "mounts[0].type must be"},
		{"bridge without name", base(`, "network": {"mode": "bridge"}`), "network.bridge is required"},
		{"bad network mode", base(`, "network": {"mode": "vlan"}`), "network.mode must be"},
		{"ip with bridge", base(`, "network": {"mode": "bridge", "bridge": "br0", "ip": "192.168.122.5"}`), "network.ip requires"},
		{"bad network ip", base(`, "network": {"mode": "default", "ip": "not-an-ip"}`), "IPv4"},
		{"bad network mac", base(`, "network": {"mode": "default", "mac": "nope"}`), "MAC address"},
		{"bad graphics type", base(`, "graphics": {"type": "sdl"}`), "graphics.type must be"},
		{"extra disk no path", base(`, "extra_disks": [{}]`), "extra_disks[0].path is required"},
		{"bad extra disk device", base(`, "extra_disks": [{"path": "/tmp/x", "device": "floppy"}]`), "extra_disks[0].device must be"},
		{"bad extra disk format", base(`, "extra_disks": [{"path": "/tmp/x", "format": "vmdk"}]`), "extra_disks[0].format must be"},
		{"provisioner without cloud image", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 5}, "provisioners": [{"type": "shell", "inline": "true"}]}`, "not supported"},
		{"provisioner both sources", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 5}, "provisioners": [{"type": "shell", "path": "/tmp/a", "inline": "true"}]}`, "exactly one of path or inline"},
		{"provisioner without source", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 5}, "provisioners": [{"type": "shell"}]}`, "exactly one of path or inline"},
		{"provisioner bad run", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 5}, "provisioners": [{"type": "shell", "inline": "true", "run": "sometimes"}]}`, "run must be"},
		{"provisioner bad type", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 5}, "provisioners": [{"type": "ansible", "inline": "true"}]}`, "type must be"},
		{"upload missing source", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 5}, "provisioners": [{"type": "shell", "inline": "true", "uploads": [{"target": "/tmp/x"}]}]}`, "uploads[0].source is required"},
		{"upload missing target", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 5}, "provisioners": [{"type": "shell", "inline": "true", "uploads": [{"source": "/tmp/x"}]}]}`, "uploads[0].target is required"},
		{"bad env key", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 5}, "provisioners": [{"type": "shell", "inline": "true", "env": {"BAD-KEY": "x"}}]}`, "env key"},
		{"provisioner user network", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x"}, "disk": {"size": 5}, "network": {"mode": "user"}, "provisioners": [{"type": "shell", "inline": "true"}]}`, "guest-visible network"},
		{"installer on cloud image", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "cloud_image", "path": "/tmp/x", "installer": {"type": "ubuntu_autoinstall"}}, "disk": {"size": 5}}`, "source.installer requires"},
		{"unknown installer", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso", "path": "/tmp/x.iso", "installer": {"type": "nope"}}, "disk": {"size": 5}}`, "unsupported installer.type"},
		{"installer config and inline", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso", "path": "/tmp/x.iso", "installer": {"type": "ubuntu_autoinstall", "config": "/tmp/a", "inline": "x"}}, "disk": {"size": 5}}`, "at most one"},
		{"installer missing type", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso", "path": "/tmp/x.iso", "installer": {}}, "disk": {"size": 5}}`, "installer.type is required"},
		{"manual iso provisioners", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso", "path": "/tmp/x.iso"}, "disk": {"size": 5}, "provisioners": [{"type": "shell", "inline": "true"}]}`, "ssh.key is required for manual ISO provisioning"},
		{"disk provisioners without key", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "disk", "path": "/tmp/x.qcow2"}, "disk": {"size": 5}, "provisioners": [{"type": "shell", "inline": "true"}]}`, "ssh.key is required for disk provisioning"},
		{"secure boot without uefi", base(`, "secure_boot": true`), "secure_boot requires"},
		{"tpm without uefi", base(`, "tpm": {"version": "2.0"}`), "tpm requires"},
		{"bad tpm version", base(`, "firmware": "uefi", "tpm": {"version": "3.0"}`), "tpm.version must be"},
		{"bad tpm backend", base(`, "firmware": "uefi", "tpm": {"backend": "passthrough"}`), "tpm.backend must be"},
		{"bad disk bus", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 5, "bus": "ide"}}`, "disk.bus must be"},
		{"windows installer without ssh", `{"name": "win", "memory": 4096, "cpus": 2, "source": {"type": "iso", "path": "/tmp/win.iso", "installer": {"type": "windows_autounattend"}}, "disk": {"size": 64, "bus": "sata"}}`, "requires ssh.key or a provisioner"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfigFile(t, tc.content)
			_, err := Load(path)
			if err == nil {
				t.Fatal("LoadConfig succeeded, want error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadConfigISOInstaller(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "auto",
		"memory": 2048,
		"cpus": 2,
		"source": {
			"type": "iso",
			"path": "/tmp/ubuntu.iso",
			"installer": {"type": "ubuntu_autoinstall", "inline": "#cloud-config\n"}
		},
		"disk": {"size": 10},
		"ssh": {"user": "deploy"},
		"provisioners": [{"type": "shell", "inline": "true"}]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.HasInstaller() || cfg.Source.Installer.Type != "ubuntu_autoinstall" {
		t.Fatalf("installer = %+v", cfg.Source.Installer)
	}
	if cfg.SSHUser() != "deploy" {
		t.Fatalf("ssh user = %q", cfg.SSHUser())
	}
	if cfg.NeedsSeed() {
		t.Fatal("ISO installer must not use the cloud-image seed path")
	}
}

func TestInstallerAnswerFileRelative(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "autoinstall.yaml"), []byte("autoinstall:\n  version: 1\n"), 0o644); err != nil {
		t.Fatalf("write answer file: %v", err)
	}
	cfg := &Config{
		Name:      "auto",
		Source:    Source{Type: SourceISO, Path: "/tmp/ubuntu.iso", Installer: &Installer{Type: "ubuntu_autoinstall", Config: "autoinstall.yaml"}},
		ConfigDir: dir,
	}
	got, err := cfg.InstallerAnswerFile()
	if err != nil {
		t.Fatalf("installerAnswerFile: %v", err)
	}
	if !strings.Contains(string(got), "version: 1") {
		t.Fatalf("answer file = %q", got)
	}
}

func TestLoadConfigManualISOAndDiskProvisioning(t *testing.T) {
	cases := []string{
		`{"name": "manual", "memory": 1024, "cpus": 1, "source": {"type": "iso", "path": "/tmp/x.iso"}, "disk": {"size": 5}, "ssh": {"user": "deploy", "key": "/tmp/id_ed25519"}, "provisioners": [{"type": "shell", "inline": "true"}]}`,
		`{"name": "disk", "memory": 1024, "cpus": 1, "source": {"type": "disk", "path": "/tmp/x.qcow2"}, "disk": {"size": 5}, "ssh": {"user": "deploy", "key": "/tmp/id_ed25519"}, "provisioners": [{"type": "shell", "inline": "true"}]}`,
	}
	for _, content := range cases {
		cfg, err := Load(writeConfigFile(t, content))
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.SSH.Key != "/tmp/id_ed25519" || cfg.SSH.User != "deploy" {
			t.Fatalf("ssh = %+v", cfg.SSH)
		}
	}
}

func TestLoadConfigWindowsDevices(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "win11",
		"memory": 4096,
		"cpus": 2,
		"os_variant": "win11",
		"firmware": "uefi",
		"secure_boot": true,
		"tpm": {"version": "2.0"},
		"source": {"type": "none"},
		"disk": {"size": 64, "format": "qcow2", "bus": "sata"}
	}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.SecureBoot || cfg.TPM == nil || cfg.TPM.Version != TPMVersion20 {
		t.Fatalf("firmware/tpm = %+v %+v", cfg.SecureBoot, cfg.TPM)
	}
	if cfg.DiskBus() != DiskBusSATA {
		t.Fatalf("diskBus = %q", cfg.DiskBus())
	}
}

func TestLoadConfigWindowsInstaller(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "win11",
		"memory": 4096,
		"cpus": 2,
		"os_variant": "win11",
		"firmware": "uefi",
		"secure_boot": true,
		"tpm": {"version": "2.0"},
		"source": {
			"type": "iso",
			"path": "/tmp/win11.iso",
			"installer": {"type": "windows_autounattend", "inline": "<unattend/>"}
		},
		"disk": {"size": 64, "bus": "sata"},
		"provisioners": [{"type": "powershell", "inline": "Write-Output hi"}]
	}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SSHUser() != "Administrator" {
		t.Fatalf("sshUser = %q", cfg.SSHUser())
	}
	spec, err := cfg.InstallerSpec()
	if err != nil {
		t.Fatalf("installerSpec: %v", err)
	}
	if spec.DiskBus != "sata" || spec.Completion != "ssh" {
		t.Fatalf("spec = %+v", spec)
	}
}

func TestWindowsGeneratedAutounattend(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "win11",
		"memory": 4096,
		"cpus": 2,
		"firmware": "uefi",
		"source": {
			"type": "iso",
			"path": "/tmp/win11.iso",
			"installer": {
				"type": "windows_autounattend",
				"windows": {"image_index": 2, "timezone": "UTC"}
			}
		},
		"disk": {"size": 64, "bus": "sata"},
		"provisioners": [{"type": "powershell", "inline": "Write-Output hi"}]
	}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.AssetDir = t.TempDir()
	cfg.SSHPublicKey = "ssh-ed25519 AAAA test"
	spec, err := cfg.InstallerSpec()
	if err != nil {
		t.Fatalf("installerSpec: %v", err)
	}
	answer := string(spec.Seed.Files[0].Data)
	for _, want := range []string{"<Value>2</Value>", "Add-WindowsCapability", "ssh-ed25519 AAAA test", "<AdministratorPassword>"} {
		if !strings.Contains(answer, want) {
			t.Errorf("autounattend missing %q", want)
		}
	}
	passwordPath := filepath.Join(cfg.AssetDir, "windows-admin-password")
	data, err := os.ReadFile(passwordPath)
	if err != nil {
		t.Fatalf("read generated password: %v", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Fatal("generated password is empty")
	}
	info, err := os.Stat(passwordPath)
	if err != nil {
		t.Fatalf("stat generated password: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("password mode = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadConfigMediaURLs(t *testing.T) {
	sha := strings.Repeat("a", 64)
	for _, sourceType := range []string{SourceISO, SourceDisk} {
		path := writeConfigFile(t, `{
			"name": "media",
			"memory": 1024,
			"cpus": 1,
			"source": {
				"type": "`+sourceType+`",
				"url": "https://example.invalid/media",
				"sha256": "`+sha+`",
				"cache": "/tmp/vmlite-media-cache"
			},
			"disk": {"size": 5}
		}`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("%s: LoadConfig: %v", sourceType, err)
		}
		if cfg.Source.URL == "" || cfg.Source.SHA256 != sha || cfg.Source.Cache == "" {
			t.Fatalf("%s: source = %+v", sourceType, cfg.Source)
		}
	}
}

func TestSampleConfig(t *testing.T) {
	cfg, err := Load("../../vm.json")
	if err != nil {
		t.Fatalf("Load(vm.json): %v", err)
	}
	if cfg.Source.Type != SourceNone {
		t.Fatalf("sample source.type = %q, want %q", cfg.Source.Type, SourceNone)
	}
	if cfg.Disk.Size <= 0 {
		t.Fatalf("sample disk.size = %d", cfg.Disk.Size)
	}
}

func TestLoadConfigURLImage(t *testing.T) {
	sha := strings.Repeat("a", 64)
	path := writeConfigFile(t, `{
		"name": "url-vm",
		"memory": 1024,
		"cpus": 1,
		"source": {
			"type": "cloud_image",
			"url": "https://example.invalid/ubuntu.qcow2",
			"sha256": "`+sha+`",
			"cache": "/tmp/vmlite-cache"
		},
		"disk": {"size": 5}
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Source.URL == "" || cfg.Source.SHA256 != sha || cfg.Source.Cache != "/tmp/vmlite-cache" {
		t.Fatalf("source = %+v", cfg.Source)
	}
}

func TestLoadConfigProvisioners(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "provisioned",
		"memory": 1024,
		"cpus": 1,
		"source": {"type": "cloud_image", "path": "/tmp/base.qcow2"},
		"disk": {"size": 5},
		"ssh": {"user": "deploy"},
		"provisioners": [
			{"type": "shell", "inline": "echo once", "run": "once"},
			{"type": "shell", "path": "./always.sh", "run": "always"}
		]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SSH.User != "deploy" {
		t.Fatalf("SSH.User = %q", cfg.SSH.User)
	}
	if len(cfg.Provisioners) != 2 || cfg.Provisioners[1].Run != "always" {
		t.Fatalf("Provisioners = %+v", cfg.Provisioners)
	}
}

func TestLoadConfigPowerShellProvisioner(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "win",
		"memory": 4096,
		"cpus": 2,
		"source": {"type": "cloud_image", "path": "/tmp/base.qcow2"},
		"disk": {"size": 20},
		"provisioners": [{"type": "powershell", "inline": "Write-Output hi"}]
	}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Provisioners) != 1 || cfg.Provisioners[0].Type != "powershell" {
		t.Fatalf("provisioners = %+v", cfg.Provisioners)
	}
}

func TestLoadConfigProvisionerUploadsEnv(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "uploads",
		"memory": 1024,
		"cpus": 1,
		"source": {"type": "cloud_image", "path": "/tmp/base.qcow2"},
		"disk": {"size": 5},
		"provisioners": [{
			"type": "shell",
			"inline": "test -f /etc/app.conf",
			"uploads": [{"source": "./app.conf", "target": "/etc/app.conf"}],
			"env": {"APP_ENV": "dev", "RETRIES": "3"}
		}]
	}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	p := cfg.Provisioners[0]
	if len(p.Uploads) != 1 || p.Uploads[0].Target != "/etc/app.conf" {
		t.Fatalf("uploads = %+v", p.Uploads)
	}
	if p.Env["APP_ENV"] != "dev" {
		t.Fatalf("env = %+v", p.Env)
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	path := writeConfigFile(t, `{not json`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("LoadConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("error = %q, want parse context", err)
	}
}

func TestLoadConfigTrailingData(t *testing.T) {
	path := writeConfigFile(t, `{"name":"vm","memory":1024,"cpus":1,"source":{"type":"none"},"disk":{"size":5}} {"extra":true}`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("LoadConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("error = %q, want trailing data", err)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	_, err := Load(path)
	if err == nil {
		t.Fatal("LoadConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "open config") {
		t.Fatalf("error = %q, want open context", err)
	}
}

func TestHashIgnoresRuntimeFields(t *testing.T) {
	cfg := Config{
		Name:   "dev",
		Memory: 1024,
		CPUs:   1,
		Source: Source{Type: SourceNone},
		Disk:   Disk{Size: 5},
	}
	before, err := Hash(&cfg)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	cfg.AssetDir = "/tmp/assets"
	cfg.ConfigDir = "/tmp/config"
	cfg.StateDir = "/tmp/state"
	cfg.Autostart = true
	cfg.SSHPublicKey = "ssh-ed25519 AAAA"
	cfg.SSHKeyGenerated = true
	cfg.WindowsPassword = "secret"
	after, err := Hash(&cfg)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if before != after {
		t.Fatal("runtime-only fields changed the config hash")
	}
}

func TestURLSourceSHA256ErrorMentionsSums(t *testing.T) {
	cfg := Config{
		Name:   "dev",
		Memory: 1024,
		CPUs:   1,
		Source: Source{Type: SourceCloudImage, URL: "https://example.invalid/image.qcow2"},
		Disk:   Disk{Size: 5},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("Validate = %v, want SHA256SUMS hint", err)
	}
}

func TestLoadConfigStaticNetwork(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "static",
		"memory": 1024,
		"cpus": 1,
		"source": {"type": "none"},
		"disk": {"size": 5},
		"network": {"mode": "default", "ip": "192.168.122.50", "mac": "52:54:00:aa:bb:cc"}
	}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Network.IP != "192.168.122.50" || cfg.Network.MAC != "52:54:00:aa:bb:cc" {
		t.Fatalf("network = %+v", cfg.Network)
	}
}

package main

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
		"extra_disks": [
			{"path": "/tmp/config.iso", "device": "cdrom", "format": "raw", "readonly": true}
		]
	}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Name != "test-vm" || cfg.Memory != 2048 || cfg.CPUs != 2 {
		t.Errorf("basic fields = %+v", cfg)
	}
	if cfg.OSVariant != "debian12" || cfg.Firmware != "uefi" {
		t.Errorf("os/firmware = %q/%q", cfg.OSVariant, cfg.Firmware)
	}
	if cfg.Source.Type != sourceCloudImage || cfg.Source.Path != "/tmp/base.qcow2" || cfg.Source.CloudInit != "/tmp/user-data" {
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
		{"iso no path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "iso"}, "disk": {"size": 5}}`, "source.path is required"},
		{"disk no path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "disk"}, "disk": {"size": 5}}`, "source.path is required"},
		{"disk with disk path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "disk", "path": "/tmp/x"}, "disk": {"size": 5, "path": "/tmp/y"}}`, "disk.path is not used"},
		{"pxe with path", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "pxe", "path": "/tmp/x"}, "disk": {"size": 5}}`, "does not accept path"},
		{"bad firmware", base(`, "firmware": "coreboot"`), "firmware must be"},
		{"bad disk format", `{"name": "vm", "memory": 1024, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 5, "format": "vmdk"}}`, "disk.format must be"},
		{"mount missing source", base(`, "mounts": [{"target": "data"}]`), "mounts[0].source is required"},
		{"mount missing target", base(`, "mounts": [{"source": "/tmp/data"}]`), "mounts[0].target is required"},
		{"bad mount type", base(`, "mounts": [{"source": "/tmp/data", "target": "data", "type": "nfs"}]`), "mounts[0].type must be"},
		{"bridge without name", base(`, "network": {"mode": "bridge"}`), "network.bridge is required"},
		{"bad network mode", base(`, "network": {"mode": "vlan"}`), "network.mode must be"},
		{"bad graphics type", base(`, "graphics": {"type": "sdl"}`), "graphics.type must be"},
		{"extra disk no path", base(`, "extra_disks": [{}]`), "extra_disks[0].path is required"},
		{"bad extra disk device", base(`, "extra_disks": [{"path": "/tmp/x", "device": "floppy"}]`), "extra_disks[0].device must be"},
		{"bad extra disk format", base(`, "extra_disks": [{"path": "/tmp/x", "format": "vmdk"}]`), "extra_disks[0].format must be"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfigFile(t, tc.content)
			_, err := LoadConfig(path)
			if err == nil {
				t.Fatal("LoadConfig succeeded, want error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestSampleConfig(t *testing.T) {
	cfg, err := LoadConfig("vm.json")
	if err != nil {
		t.Fatalf("LoadConfig(vm.json): %v", err)
	}
	if cfg.Source.Type != sourceNone {
		t.Fatalf("sample source.type = %q, want %q", cfg.Source.Type, sourceNone)
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

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Source.URL == "" || cfg.Source.SHA256 != sha || cfg.Source.Cache != "/tmp/vmlite-cache" {
		t.Fatalf("source = %+v", cfg.Source)
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	path := writeConfigFile(t, `{not json`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("error = %q, want parse context", err)
	}
}

func TestLoadConfigTrailingData(t *testing.T) {
	path := writeConfigFile(t, `{"name":"vm","memory":1024,"cpus":1,"source":{"type":"none"},"disk":{"size":5}} {"extra":true}`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("error = %q, want trailing data", err)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig succeeded, want error")
	}
	if !strings.Contains(err.Error(), "open config") {
		t.Fatalf("error = %q, want open context", err)
	}
}

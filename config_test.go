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
		"disk_size": 20,
		"os_variant": "debian12",
		"install_url": "https://example.invalid/debian/",
		"mounts": [{"source": "/tmp/data", "target": "shared_data"}]
	}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Name != "test-vm" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-vm")
	}
	if cfg.Memory != 2048 {
		t.Errorf("Memory = %d, want 2048", cfg.Memory)
	}
	if cfg.CPUs != 2 {
		t.Errorf("CPUs = %d, want 2", cfg.CPUs)
	}
	if cfg.DiskSize != 20 {
		t.Errorf("DiskSize = %d, want 20", cfg.DiskSize)
	}
	if cfg.OSVariant != "debian12" {
		t.Errorf("OSVariant = %q, want %q", cfg.OSVariant, "debian12")
	}
	if cfg.InstallURL != "https://example.invalid/debian/" {
		t.Errorf("InstallURL = %q", cfg.InstallURL)
	}
	if len(cfg.Mounts) != 1 || cfg.Mounts[0].Source != "/tmp/data" || cfg.Mounts[0].Target != "shared_data" {
		t.Errorf("Mounts = %+v", cfg.Mounts)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing name",
			content: `{"memory": 1024, "cpus": 1, "disk_size": 5, "os_variant": "debian12"}`,
			wantErr: "name is required",
		},
		{
			name:    "zero memory",
			content: `{"name": "vm", "memory": 0, "cpus": 1, "disk_size": 5, "os_variant": "debian12"}`,
			wantErr: "memory must be greater than zero",
		},
		{
			name:    "negative cpus",
			content: `{"name": "vm", "memory": 1024, "cpus": -1, "disk_size": 5, "os_variant": "debian12"}`,
			wantErr: "cpus must be greater than zero",
		},
		{
			name:    "zero disk",
			content: `{"name": "vm", "memory": 1024, "cpus": 1, "disk_size": 0, "os_variant": "debian12"}`,
			wantErr: "disk_size must be greater than zero",
		},
		{
			name:    "missing os variant",
			content: `{"name": "vm", "memory": 1024, "cpus": 1, "disk_size": 5}`,
			wantErr: "os_variant is required",
		},
		{
			name:    "mount missing source",
			content: `{"name": "vm", "memory": 1024, "cpus": 1, "disk_size": 5, "os_variant": "debian12", "mounts": [{"target": "data"}]}`,
			wantErr: "mounts[0].source is required",
		},
		{
			name:    "mount missing target",
			content: `{"name": "vm", "memory": 1024, "cpus": 1, "disk_size": 5, "os_variant": "debian12", "mounts": [{"source": "/tmp/data"}]}`,
			wantErr: "mounts[0].target is required",
		},
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

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWritesDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vm.json")
	if err := runInit([]string{path}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg["name"] != "cloud-dev" {
		t.Fatalf("name = %v", cfg["name"])
	}
}

func TestInitStdoutAndName(t *testing.T) {
	out, err := captureCommandOutput(t, func() error {
		return runInit([]string{"--stdout", "--template", "minimal", "--name", "demo"})
	})
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("parse stdout: %v\n%s", err, out)
	}
	if cfg["name"] != "demo" {
		t.Fatalf("name = %v", cfg["name"])
	}
}

func TestInitList(t *testing.T) {
	out, err := captureCommandOutput(t, func() error {
		return runInit([]string{"--list"})
	})
	if err != nil {
		t.Fatalf("runInit --list: %v", err)
	}
	for _, want := range []string{"cloud-image", "minimal", "arch-script"} {
		if !strings.Contains(out, want) {
			t.Errorf("template list missing %q:\n%s", want, out)
		}
	}
}

func TestInitRefusesOverwriteAndForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vm.json")
	if err := os.WriteFile(path, []byte(`{"old": true}`), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	if err := runInit([]string{path}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("runInit = %v, want exists error", err)
	}
	if err := runInit([]string{"--force", path}); err != nil {
		t.Fatalf("runInit --force: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), `"old"`) {
		t.Fatalf("config was not overwritten:\n%s", data)
	}
}

func TestInitUnknownTemplate(t *testing.T) {
	err := runInit([]string{"--stdout", "--template", "does-not-exist"})
	if err == nil || !strings.Contains(err.Error(), "unknown template") {
		t.Fatalf("runInit = %v, want unknown template", err)
	}
}

func TestInitNestedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project", "vm.json")
	if err := runInit([]string{"--template", "minimal", path}); err != nil {
		t.Fatalf("runInit nested: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nested config missing: %v", err)
	}
}

func TestInitMinimalConfigLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vm.json")
	if err := runInit([]string{"--template", "minimal", path}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("generated config is invalid: %v", err)
	}
}

func TestInitCloudImageConfigLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vm.json")
	if err := runInit([]string{"--template", "cloud-image", path}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("generated config is invalid: %v", err)
	}
	if cfg.Source.URL == "" || cfg.Source.SHA256 == "" {
		t.Fatalf("cloud-image source = %+v", cfg.Source)
	}
}

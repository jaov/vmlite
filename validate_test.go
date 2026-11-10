package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCommand(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "valid",
		"memory": 512,
		"cpus": 1,
		"source": {"type": "none"},
		"disk": {"size": 5}
	}`)
	if err := runValidate([]string{path}); err != nil {
		t.Fatalf("runValidate: %v", err)
	}
}

func TestValidateCommandMissingPaths(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, `{
		"name": "missing",
		"memory": 512,
		"cpus": 1,
		"source": {"type": "iso", "path": "`+filepath.Join(dir, "missing.iso")+`"},
		"disk": {"size": 5}
	}`)
	err := runValidate([]string{path})
	if err == nil || !strings.Contains(err.Error(), "source.path") || !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("runValidate = %v, want source.path error", err)
	}
}

func TestValidateCommandMissingSSHKey(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, `{
		"name": "missing-key",
		"memory": 512,
		"cpus": 1,
		"source": {"type": "disk", "path": "`+filepath.Join(dir, "disk.qcow2")+`"},
		"disk": {"size": 5, "format": "raw"},
		"ssh": {"key": "`+filepath.Join(dir, "id_ed25519")+`"},
		"provisioners": [{"type": "shell", "inline": "true"}]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "disk.qcow2"), []byte("disk"), 0o644); err != nil {
		t.Fatalf("write disk: %v", err)
	}
	err := runValidate([]string{path})
	if err == nil || !strings.Contains(err.Error(), "ssh.key") {
		t.Fatalf("runValidate = %v, want ssh.key error", err)
	}
}

func TestValidateCommandInvalidConfig(t *testing.T) {
	path := writeConfigFile(t, `{"name": "", "memory": 512, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 5}}`)
	if err := runValidate([]string{path}); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("runValidate = %v, want validation error", err)
	}
}

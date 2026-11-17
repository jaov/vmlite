package state

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestProjectLayout(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "vm.json")

	layout, err := NewProjectLayout(configPath, "", "dev")
	if err != nil {
		t.Fatalf("NewProjectLayout: %v", err)
	}
	if layout.StateDir != filepath.Join(dir, ".vmlite") {
		t.Fatalf("StateDir = %q", layout.StateDir)
	}
	if layout.StateFile != filepath.Join(dir, ".vmlite", "dev.json") {
		t.Fatalf("StateFile = %q", layout.StateFile)
	}
	if layout.AssetDir != filepath.Join(dir, ".vmlite", "dev") {
		t.Fatalf("AssetDir = %q", layout.AssetDir)
	}
}

func TestStateSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := &State{
		Version:    1,
		Name:       "dev",
		ConfigPath: "/project/vm.json",
		ConfigHash: "abc",
		LibvirtURI: "qemu:///system",
		UUID:       "uuid",
		CreatedAt:  "now",
		UpdatedAt:  "now",
	}
	if err := Save(path, st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || !reflect.DeepEqual(*got, *st) {
		t.Fatalf("state = %+v, want %+v", got, st)
	}
}

func TestLoadMissing(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || got != nil {
		t.Fatalf("Load missing = %+v, %v", got, err)
	}
}

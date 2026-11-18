package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func targetConfig(t *testing.T, dir, name, stateDir string) string {
	t.Helper()
	stateField := ""
	if stateDir != "" {
		stateField = fmt.Sprintf(",\n  \"state_dir\": %q", stateDir)
	}
	path := filepath.Join(dir, "vm.json")
	content := fmt.Sprintf(`{
  "name": %q,
  "memory": 256,
  "cpus": 1,
  "source": {"type": "none"},
  "disk": {"size": 1}%s
}`, name, stateField)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func targetState(t *testing.T, layout projectLayout, name string) {
	t.Helper()
	if err := saveState(layout.StateFile, &State{
		Version:    1,
		Name:       name,
		ConfigPath: layout.ConfigPath,
		LibvirtURI: systemURI,
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func TestResolveConfigStateDirPrecedence(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	configState := filepath.Join(dir, "config-state")
	path := targetConfig(t, dir, "precedence", configState)

	_, layout, err := resolveConfig(path, "")
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	if layout.StateDir != configState {
		t.Fatalf("state dir = %q, want %q", layout.StateDir, configState)
	}

	envState := filepath.Join(dir, "env-state")
	t.Setenv("VMLITE_STATE_DIR", envState)
	_, layout, err = resolveConfig(path, "")
	if err != nil {
		t.Fatalf("resolveConfig env: %v", err)
	}
	if layout.StateDir != envState {
		t.Fatalf("env state dir = %q, want %q", layout.StateDir, envState)
	}

	flagState := filepath.Join(dir, "flag-state")
	_, layout, err = resolveConfig(path, flagState)
	if err != nil {
		t.Fatalf("resolveConfig flag: %v", err)
	}
	if layout.StateDir != flagState {
		t.Fatalf("flag state dir = %q, want %q", layout.StateDir, flagState)
	}
}

func TestResolveConfigRelativeStateDir(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	path := targetConfig(t, dir, "relative", "state")

	_, layout, err := resolveConfig(path, "")
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	want := filepath.Join(dir, "state")
	if layout.StateDir != want {
		t.Fatalf("state dir = %q, want %q", layout.StateDir, want)
	}
}

func TestResolveConfigDefaultStateDir(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	path := targetConfig(t, dir, "default", "")

	_, layout, err := resolveConfig(path, "")
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	want := filepath.Join(dir, ".vmlite")
	if layout.StateDir != want {
		t.Fatalf("state dir = %q, want %q", layout.StateDir, want)
	}
}

func TestResolveVMByName(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	path := targetConfig(t, dir, "by-name", stateDir)
	_, layout, err := resolveConfig(path, "")
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	targetState(t, layout, "by-name")
	chdir(t, dir)

	got, err := resolveVM("by-name", "")
	if err != nil {
		t.Fatalf("resolveVM: %v", err)
	}
	if got.State == nil || got.State.Name != "by-name" {
		t.Fatalf("state = %+v", got.State)
	}
	if got.Layout.StateFile != layout.StateFile {
		t.Fatalf("state file = %q, want %q", got.Layout.StateFile, layout.StateFile)
	}
}

func TestResolveVMNameUsesFlagAndEnv(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	path := targetConfig(t, dir, "by-state-dir", "")
	_, layout, err := resolveConfig(path, stateDir)
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	targetState(t, layout, "by-state-dir")

	if _, err := resolveVM("by-state-dir", stateDir); err != nil {
		t.Fatalf("resolveVM flag: %v", err)
	}

	t.Setenv("VMLITE_STATE_DIR", stateDir)
	if _, err := resolveVM("by-state-dir", ""); err != nil {
		t.Fatalf("resolveVM env: %v", err)
	}
}

func TestResolveVMUnknownName(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	targetConfig(t, dir, "known", filepath.Join(dir, "state"))
	chdir(t, dir)

	_, err := resolveVM("missing", "")
	if err == nil || !strings.Contains(err.Error(), "no VM named") {
		t.Fatalf("resolveVM = %v, want no VM error", err)
	}
}

func TestResolveVMRejectsMismatchedState(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	path := targetConfig(t, dir, "expected", stateDir)
	_, layout, err := resolveConfig(path, "")
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	targetState(t, layout, "other")
	chdir(t, dir)

	_, err = resolveVM("expected", "")
	if err == nil || !strings.Contains(err.Error(), "belongs to VM") {
		t.Fatalf("resolveVM = %v, want mismatch error", err)
	}
}

func TestResolveVMMissingState(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	path := targetConfig(t, dir, "missing-state", "")

	got, err := resolveVM(path, "")
	if err != nil {
		t.Fatalf("resolveVM: %v", err)
	}
	if got.State != nil {
		t.Fatalf("state = %+v, want nil", got.State)
	}
	if got.Config == nil || got.Config.Name != "missing-state" {
		t.Fatalf("config = %+v", got.Config)
	}
}

func TestListUsesConfiguredStateDir(t *testing.T) {
	t.Setenv("VMLITE_STATE_DIR", "")
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	path := targetConfig(t, dir, "listed", stateDir)
	_, layout, err := resolveConfig(path, "")
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	targetState(t, layout, "listed")
	chdir(t, dir)
	fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; esac\nexit 0\n")

	out, err := captureCommandOutput(t, func() error {
		return runList([]string{"--json"})
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var entries []listEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("parse list JSON: %v\n%s", err, out)
	}
	if len(entries) != 1 || entries[0].Name != "listed" {
		t.Fatalf("entries = %+v", entries)
	}
}

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectLayout(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "vm.json")

	layout, err := newProjectLayout(configPath, "", "dev")
	if err != nil {
		t.Fatalf("newProjectLayout: %v", err)
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

func TestHashConfig(t *testing.T) {
	cfg := Config{Name: "dev", Memory: 1024, CPUs: 1, Source: Source{Type: sourceNone}, Disk: Disk{Size: 5}}
	a, err := hashConfig(&cfg)
	if err != nil {
		t.Fatalf("hashConfig: %v", err)
	}
	b, err := hashConfig(&cfg)
	if err != nil {
		t.Fatalf("hashConfig: %v", err)
	}
	if a != b {
		t.Fatalf("hash is not stable: %q != %q", a, b)
	}
	cfg.Memory = 2048
	c, err := hashConfig(&cfg)
	if err != nil {
		t.Fatalf("hashConfig: %v", err)
	}
	if a == c {
		t.Fatal("hash did not change after a config change")
	}
}

func TestStateSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := &State{
		Version:    1,
		Name:       "dev",
		ConfigPath: "/project/vm.json",
		ConfigHash: "abc",
		LibvirtURI: systemURI,
		UUID:       "uuid",
		CreatedAt:  "now",
		UpdatedAt:  "now",
	}
	if err := saveState(path, st); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	got, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got == nil || *got != *st {
		t.Fatalf("state = %+v, want %+v", got, st)
	}
}

func TestApplyProjectDefaults(t *testing.T) {
	layout := projectLayout{AssetDir: "/project/.vmlite/dev"}
	cfg := Config{Source: Source{Type: sourceCloudImage, Path: "/tmp/base.qcow2"}}
	applyProjectDefaults(&cfg, layout)
	want := filepath.Join(layout.AssetDir, "disk.qcow2")
	if cfg.Disk.Path != want {
		t.Fatalf("Disk.Path = %q, want %q", cfg.Disk.Path, want)
	}

	explicit := Config{Source: Source{Type: sourceCloudImage, Path: "/tmp/base.qcow2"}, Disk: Disk{Path: "/tmp/disk.qcow2"}}
	applyProjectDefaults(&explicit, layout)
	if explicit.Disk.Path != "/tmp/disk.qcow2" {
		t.Fatalf("explicit Disk.Path changed to %q", explicit.Disk.Path)
	}

	none := Config{Source: Source{Type: sourceNone}}
	applyProjectDefaults(&none, layout)
	if none.Disk.Path != "" {
		t.Fatalf("source.none Disk.Path changed to %q", none.Disk.Path)
	}
}

func TestNewState(t *testing.T) {
	dir := t.TempDir()
	layout := projectLayout{
		ConfigPath: filepath.Join(dir, "vm.json"),
		StateDir:   filepath.Join(dir, ".vmlite"),
		StateFile:  filepath.Join(dir, ".vmlite", "dev.json"),
		AssetDir:   filepath.Join(dir, ".vmlite", "dev"),
	}
	cfg := &Config{
		Name:      "dev",
		Source:    Source{Type: sourceCloudImage, Path: "/tmp/base.qcow2", CloudInit: "/tmp/user-data"},
		Disk:      Disk{Size: 10, Path: filepath.Join(layout.AssetDir, "disk.qcow2")},
		Firmware:  firmwareBIOS,
		OSVariant: "debian12",
	}
	st, err := newState(cfg, layout, systemURI, "hash", "uuid")
	if err != nil {
		t.Fatalf("newState: %v", err)
	}
	if st.ImagePath != "/tmp/base.qcow2" {
		t.Fatalf("ImagePath = %q", st.ImagePath)
	}
	if st.DiskPath != cfg.Disk.Path {
		t.Fatalf("DiskPath = %q", st.DiskPath)
	}
	if st.SeedPath != filepath.Join(layout.AssetDir, "dev-seed.iso") {
		t.Fatalf("SeedPath = %q", st.SeedPath)
	}
	if st.PoolName != "dev" {
		t.Fatalf("PoolName = %q", st.PoolName)
	}
}

func TestStatusNoState(t *testing.T) {
	path := writeConfigFile(t, `{"name": "status-dev", "memory": 256, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 1}}`)
	err := runStatus([]string{path})
	if !errors.Is(err, errNotRunning) {
		t.Fatalf("runStatus = %v, want errNotRunning", err)
	}
}

func TestDestroyNoState(t *testing.T) {
	path := writeConfigFile(t, `{"name": "destroy-dev", "memory": 256, "cpus": 1, "source": {"type": "none"}, "disk": {"size": 1}}`)
	err := runDestroy([]string{path})
	if err == nil {
		t.Fatal("runDestroy succeeded without state")
	}
	if !strings.Contains(err.Error(), "no state") {
		t.Fatalf("error = %q, want no state", err)
	}
}

func TestWithinDir(t *testing.T) {
	dir := t.TempDir()
	if !withinDir(dir, filepath.Join(dir, "child")) {
		t.Fatal("child should be within dir")
	}
	if withinDir(dir, filepath.Join(dir, "..", "outside")) {
		t.Fatal("outside path should not be within dir")
	}
}

func TestStateFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := saveState(path, &State{Version: 1, Name: "dev"}); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("state mode = %o, want 644", info.Mode().Perm())
	}
}

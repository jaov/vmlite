package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State records the managed VM and the files vmlite created for it.
type State struct {
	Version     int    `json:"version"`
	Name        string `json:"name"`
	ConfigPath  string `json:"config_path"`
	ConfigHash  string `json:"config_hash"`
	LibvirtURI  string `json:"libvirt_uri"`
	UUID        string `json:"uuid,omitempty"`
	ImagePath   string `json:"image_path,omitempty"`
	ImageSHA256 string `json:"image_sha256,omitempty"`
	DiskPath    string `json:"disk_path,omitempty"`
	SeedPath    string `json:"seed_path,omitempty"`
	AssetDir    string `json:"asset_dir,omitempty"`
	PoolName    string `json:"pool_name,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type projectLayout struct {
	ConfigPath string
	StateDir   string
	StateFile  string
	AssetDir   string
}

func newProjectLayout(configPath, stateDirFlag, name string) (projectLayout, error) {
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return projectLayout{}, fmt.Errorf("resolve config path %q: %w", configPath, err)
	}

	stateDir := filepath.Join(filepath.Dir(absConfig), ".vmlite")
	if strings.TrimSpace(stateDirFlag) != "" {
		stateDir, err = filepath.Abs(stateDirFlag)
		if err != nil {
			return projectLayout{}, fmt.Errorf("resolve state dir %q: %w", stateDirFlag, err)
		}
	}

	return projectLayout{
		ConfigPath: absConfig,
		StateDir:   stateDir,
		StateFile:  filepath.Join(stateDir, name+".json"),
		AssetDir:   filepath.Join(stateDir, name),
	}, nil
}

func hashConfig(cfg *Config) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("hash config: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func loadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state %q: %w", path, err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse state %q: %w", path, err)
	}
	if st.Version != 1 {
		return nil, fmt.Errorf("state %q has unsupported version %d", path, st.Version)
	}
	return &st, nil
}

func saveState(path string, st *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create state temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod state temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write state temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close state temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace state %q: %w", path, err)
	}
	return nil
}

// applyProjectDefaults makes the managed disk location deterministic.
func applyProjectDefaults(cfg *Config, layout projectLayout) {
	if cfg.Source.Type == sourceCloudImage && strings.TrimSpace(cfg.Disk.Path) == "" {
		cfg.Disk.Path = filepath.Join(layout.AssetDir, "disk.qcow2")
	}
}

func newState(cfg *Config, layout projectLayout, uri, configHash, uuid string) (*State, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	st := &State{
		Version:    1,
		Name:       cfg.Name,
		ConfigPath: layout.ConfigPath,
		ConfigHash: configHash,
		LibvirtURI: uri,
		UUID:       uuid,
		AssetDir:   layout.AssetDir,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if strings.TrimSpace(cfg.Source.Path) != "" {
		image, err := filepath.Abs(cfg.Source.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve image %q: %w", cfg.Source.Path, err)
		}
		st.ImagePath = image
	}
	if strings.TrimSpace(cfg.Disk.Path) != "" {
		disk, err := filepath.Abs(cfg.Disk.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve disk %q: %w", cfg.Disk.Path, err)
		}
		st.DiskPath = disk
		if withinDir(layout.AssetDir, disk) {
			st.PoolName = filepath.Base(layout.AssetDir)
		}
	}
	if strings.TrimSpace(cfg.Source.CloudInit) != "" {
		seed, err := cfg.seedPath()
		if err != nil {
			return nil, err
		}
		st.SeedPath = seed
	}
	return st, nil
}

func withinDir(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

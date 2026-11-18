// Package state stores vmlite's project layout and persisted VM state.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// State records the managed VM and the files vmlite created for it.
type State struct {
	Version             int               `json:"version"`
	Name                string            `json:"name"`
	ConfigPath          string            `json:"config_path"`
	ConfigHash          string            `json:"config_hash"`
	LibvirtURI          string            `json:"libvirt_uri"`
	UUID                string            `json:"uuid,omitempty"`
	ImagePath           string            `json:"image_path,omitempty"`
	ImageSHA256         string            `json:"image_sha256,omitempty"`
	DiskPath            string            `json:"disk_path,omitempty"`
	SeedPath            string            `json:"seed_path,omitempty"`
	InstallerISOPath    string            `json:"installer_iso_path,omitempty"`
	AssetDir            string            `json:"asset_dir,omitempty"`
	PoolName            string            `json:"pool_name,omitempty"`
	SSHKeyPath          string            `json:"ssh_key_path,omitempty"`
	SSHKeyGenerated     bool              `json:"ssh_key_generated,omitempty"`
	SSHUser             string            `json:"ssh_user,omitempty"`
	IP                  string            `json:"ip,omitempty"`
	NetworkName         string            `json:"network_name,omitempty"`
	NetworkMAC          string            `json:"network_mac,omitempty"`
	NetworkIP           string            `json:"network_ip,omitempty"`
	InstallerType       string            `json:"installer_type,omitempty"`
	InstallerCompletion string            `json:"installer_completion,omitempty"`
	InstallPhase        string            `json:"install_phase,omitempty"`
	Provisioned         []ProvisionRecord `json:"provisioned,omitempty"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
}

// ProvisionRecord records one completed provisioner hash.
type ProvisionRecord struct {
	ID          string `json:"id"`
	SHA256      string `json:"sha256"`
	CompletedAt string `json:"completed_at"`
}

const (
	PhaseInstalling = "installing"
	PhaseInstalled  = "installed"
)

// ProjectLayout identifies a config and its managed state paths.
type ProjectLayout struct {
	ConfigPath string
	StateDir   string
	StateFile  string
	LockFile   string
	AssetDir   string
}

// NewProjectLayout resolves the state layout for a config.
func NewProjectLayout(configPath, stateDirFlag, name string) (ProjectLayout, error) {
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return ProjectLayout{}, fmt.Errorf("resolve config path %q: %w", configPath, err)
	}

	stateDir := filepath.Join(filepath.Dir(absConfig), ".vmlite")
	if strings.TrimSpace(stateDirFlag) != "" {
		stateDir, err = filepath.Abs(stateDirFlag)
		if err != nil {
			return ProjectLayout{}, fmt.Errorf("resolve state dir %q: %w", stateDirFlag, err)
		}
	}

	return ProjectLayout{
		ConfigPath: absConfig,
		StateDir:   stateDir,
		StateFile:  filepath.Join(stateDir, name+".json"),
		LockFile:   filepath.Join(stateDir, name+".lock"),
		AssetDir:   filepath.Join(stateDir, name),
	}, nil
}

// Load reads a state file. A missing file returns nil, nil.
func Load(path string) (*State, error) {
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

// Save atomically writes a state file.
func Save(path string, st *State) error {
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

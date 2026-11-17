package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	guestinstall "vmlite/internal/installer"
)

func hashConfig(cfg *Config) (string, error) {
	// Provisioning and SSH settings are not VM shape; changing a script must
	// not require recreating the domain.
	clone := *cfg
	clone.SSH = SSHConfig{}
	clone.Provisioners = nil
	data, err := json.Marshal(&clone)
	if err != nil {
		return "", fmt.Errorf("hash config: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// applyProjectDefaults makes the managed disk location deterministic.
func applyProjectDefaults(cfg *Config, layout projectLayout) {
	cfg.assetDir = layout.AssetDir
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
	if cfg.Source.SHA256 != "" {
		st.ImageSHA256 = strings.ToLower(cfg.Source.SHA256)
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
	if cfg.needsSeed() {
		seed, err := cfg.seedPath()
		if err != nil {
			return nil, err
		}
		st.SeedPath = seed
	}
	if cfg.hasInstaller() {
		st.InstallerType = cfg.Source.Installer.Type
		st.InstallPhase = installPhaseInstalling
		spec, err := cfg.installerSpec()
		if err != nil {
			return nil, err
		}
		completion := spec.Completion
		if completion == "" {
			completion = guestinstall.CompletionShutdown
		}
		st.InstallerCompletion = string(completion)
		if spec.Seed != nil {
			seed, err := cfg.installerSeedPath()
			if err != nil {
				return nil, err
			}
			st.SeedPath = seed
		}
		if len(spec.InjectFiles) > 0 {
			iso, err := cfg.installerISOPath()
			if err != nil {
				return nil, err
			}
			st.InstallerISOPath = iso
		}
		// virt-install may create a directory pool for the seed media's
		// directory; record it so destroy cleans it up.
		st.PoolName = filepath.Base(layout.AssetDir)
	}
	if strings.TrimSpace(cfg.SSH.Key) != "" {
		key, err := filepath.Abs(cfg.SSH.Key)
		if err != nil {
			return nil, fmt.Errorf("resolve ssh key %q: %w", cfg.SSH.Key, err)
		}
		st.SSHKeyPath = key
		st.SSHKeyGenerated = cfg.sshKeyGenerated
		st.SSHUser = cfg.sshUser()
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

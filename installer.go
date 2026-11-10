package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	guestinstall "vmlite/internal/installer"
	"vmlite/internal/seed"
)

func (c *Config) installerSpec() (guestinstall.Spec, error) {
	if !c.hasInstaller() {
		return guestinstall.Spec{}, nil
	}
	impl, err := guestinstall.Lookup(c.Source.Installer.Type)
	if err != nil {
		return guestinstall.Spec{}, err
	}
	answer, err := c.installerAnswerFile()
	if err != nil {
		return guestinstall.Spec{}, err
	}
	return impl.Spec(guestinstall.Input{
		Hostname:   c.Name,
		SSHUser:    c.sshUser(),
		SSHKey:     c.sshPublicKey,
		AnswerFile: answer,
	})
}

func (c *Config) installerAnswerFile() ([]byte, error) {
	inst := c.Source.Installer
	if inst == nil {
		return nil, nil
	}
	if strings.TrimSpace(inst.Inline) != "" {
		return []byte(inst.Inline), nil
	}
	if strings.TrimSpace(inst.Config) == "" {
		return nil, nil
	}

	path := inst.Config
	if !filepath.IsAbs(path) {
		if c.configDir != "" {
			path = filepath.Join(c.configDir, path)
		}
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve installer config %q: %w", inst.Config, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("installer config %q is not accessible: %w", inst.Config, err)
	}
	return data, nil
}

func (c *Config) installerSeedPath() (string, error) {
	if !c.hasInstaller() {
		return "", nil
	}
	if strings.TrimSpace(c.assetDir) == "" {
		return "", fmt.Errorf("installer seed path requires the project asset directory")
	}
	return filepath.Join(c.assetDir, c.Name+"-installer-seed.iso"), nil
}

// prepareInstallerSeed creates the answer media consumed by an unattended ISO
// installer. Cloud images use prepareSeed instead.
func prepareInstallerSeed(cfg *Config) (string, error) {
	spec, err := cfg.installerSpec()
	if err != nil {
		return "", err
	}
	if spec.Seed == nil {
		return "", nil
	}
	path, err := cfg.installerSeedPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check installer seed %q: %w", path, err)
	}
	if err := seed.WriteISO(*spec.Seed, path); err != nil {
		return "", err
	}
	return path, nil
}

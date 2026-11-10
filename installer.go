package main

import (
	"fmt"
	"os"
	"os/exec"
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

// installerISOPath returns the installer media path. Installers that need
// files injected use a remastered copy in the project asset directory.
func (c *Config) installerISOPath() (string, error) {
	source, err := filepath.Abs(c.Source.Path)
	if err != nil {
		return "", fmt.Errorf("resolve iso %q: %w", c.Source.Path, err)
	}
	spec, err := c.installerSpec()
	if err != nil {
		return "", err
	}
	if len(spec.InjectFiles) == 0 {
		return source, nil
	}
	if strings.TrimSpace(c.assetDir) == "" {
		return "", fmt.Errorf("remastered installer ISO requires the project asset directory")
	}
	return filepath.Join(c.assetDir, c.Name+"-installer.iso"), nil
}

// prepareInstallerISO remasters the source ISO with installer.Spec.InjectFiles.
func prepareInstallerISO(cfg *Config, spec guestinstall.Spec) (string, error) {
	if len(spec.InjectFiles) == 0 {
		return "", nil
	}
	path, err := cfg.installerISOPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check remastered installer ISO %q: %w", path, err)
	}
	source, err := filepath.Abs(cfg.Source.Path)
	if err != nil {
		return "", fmt.Errorf("resolve iso %q: %w", cfg.Source.Path, err)
	}
	if _, err := os.Stat(source); err != nil {
		return "", fmt.Errorf("iso %q is not accessible: %w", cfg.Source.Path, err)
	}
	if _, err := exec.LookPath("xorriso"); err != nil {
		return "", fmt.Errorf("remastering installer media requires xorriso in PATH: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create installer ISO directory: %w", err)
	}

	dir, err := os.MkdirTemp("", "vmlite-iso-")
	if err != nil {
		return "", fmt.Errorf("create ISO workspace: %w", err)
	}
	defer os.RemoveAll(dir)

	tmp, err := os.CreateTemp(filepath.Dir(path), ".installer-*.iso")
	if err != nil {
		return "", fmt.Errorf("create remastered ISO temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	os.Remove(tmpPath)
	defer os.Remove(tmpPath)

	args := []string{"-indev", source, "-outdev", tmpPath}
	for _, f := range spec.InjectFiles {
		clean := filepath.Clean(f.Name)
		if f.Name == "" || filepath.IsAbs(f.Name) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("injected installer file %q must be a relative path", f.Name)
		}
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		filePath := filepath.Join(dir, clean)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return "", fmt.Errorf("create injected installer directory for %q: %w", f.Name, err)
		}
		if err := os.WriteFile(filePath, f.Data, mode); err != nil {
			return "", fmt.Errorf("write injected installer file %q: %w", f.Name, err)
		}
		args = append(args, "-map", filePath, "/"+filepath.ToSlash(clean))
	}
	args = append(args, "-boot_image", "any", "replay")

	cmd := exec.Command("xorriso", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("remaster installer ISO %q: %w: %s", cfg.Source.Path, err, strings.TrimSpace(string(out)))
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return "", fmt.Errorf("chmod remastered ISO: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("install remastered ISO %q: %w", path, err)
	}
	return path, nil
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

package main

import (
	"crypto/rand"
	"encoding/hex"
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
	input := guestinstall.Input{
		Hostname:   c.Name,
		SSHUser:    c.sshUser(),
		SSHKey:     c.sshPublicKey,
		AnswerFile: answer,
	}
	if c.Source.Installer.Type == guestinstall.WindowsAutoUnattend {
		windows := guestinstall.WindowsOptions{Firmware: c.Firmware}
		if inst := c.Source.Installer.Windows; inst != nil {
			windows.Locale = inst.Locale
			windows.InputLocale = inst.InputLocale
			windows.Timezone = inst.Timezone
			windows.ImageIndex = inst.ImageIndex
			windows.AdminPassword = inst.AdminPassword
		}
		if len(answer) == 0 && strings.TrimSpace(windows.AdminPassword) == "" {
			password, err := c.windowsAdminPassword()
			if err != nil {
				return guestinstall.Spec{}, err
			}
			windows.AdminPassword = password
		}
		input.Windows = &windows
	}
	return impl.Spec(input)
}

func (c *Config) windowsAdminPassword() (string, error) {
	if c.windowsPassword != "" {
		return c.windowsPassword, nil
	}
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate Windows administrator password: %w", err)
	}
	password := "A!9" + hex.EncodeToString(raw)
	c.windowsPassword = password
	if strings.TrimSpace(c.assetDir) != "" {
		if err := os.MkdirAll(c.assetDir, 0o755); err != nil {
			return "", fmt.Errorf("create asset dir: %w", err)
		}
		path := filepath.Join(c.assetDir, "windows-admin-password")
		if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
			return "", fmt.Errorf("write Windows administrator password: %w", err)
		}
	}
	return password, nil
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

// prepareInstallerKernel extracts the kernel and initrd used by installers
// that boot a live ISO directly instead of using --location.
func prepareInstallerKernel(cfg *Config, spec guestinstall.Spec) (string, string, error) {
	if !spec.KernelBoot {
		return "", "", nil
	}
	if strings.TrimSpace(spec.KernelPath) == "" || strings.TrimSpace(spec.InitrdPath) == "" {
		return "", "", fmt.Errorf("installer %q uses kernel boot without kernel or initrd paths", spec.Type)
	}
	for _, path := range []string{spec.KernelPath, spec.InitrdPath} {
		clean := filepath.Clean(path)
		if !filepath.IsAbs(path) || clean != path {
			return "", "", fmt.Errorf("installer kernel path %q must be a clean absolute ISO path", path)
		}
	}
	if strings.TrimSpace(cfg.assetDir) == "" {
		return "", "", fmt.Errorf("installer kernel extraction requires the project asset directory")
	}

	iso, err := prepareInstallerISO(cfg, spec)
	if err != nil {
		return "", "", err
	}
	if iso == "" {
		iso, err = filepath.Abs(cfg.Source.Path)
		if err != nil {
			return "", "", fmt.Errorf("resolve iso %q: %w", cfg.Source.Path, err)
		}
	}
	if _, err := os.Stat(iso); err != nil {
		return "", "", fmt.Errorf("iso %q is not accessible: %w", iso, err)
	}

	kernel := filepath.Join(cfg.assetDir, "installer-vmlinuz")
	initrd := filepath.Join(cfg.assetDir, "installer-initrd")
	kernelReady := false
	if _, err := os.Stat(kernel); err == nil {
		kernelReady = true
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("check installer kernel: %w", err)
	}
	initrdReady := false
	if _, err := os.Stat(initrd); err == nil {
		initrdReady = true
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("check installer initrd: %w", err)
	}
	if kernelReady && initrdReady {
		return kernel, initrd, nil
	}

	if _, err := exec.LookPath("xorriso"); err != nil {
		return "", "", fmt.Errorf("extracting installer kernel requires xorriso in PATH: %w", err)
	}
	if err := os.MkdirAll(cfg.assetDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create installer asset directory: %w", err)
	}

	tmpKernel, err := os.CreateTemp(cfg.assetDir, ".installer-vmlinuz-*")
	if err != nil {
		return "", "", fmt.Errorf("create installer kernel temp file: %w", err)
	}
	tmpKernelPath := tmpKernel.Name()
	tmpKernel.Close()
	os.Remove(tmpKernelPath)
	defer os.Remove(tmpKernelPath)

	tmpInitrd, err := os.CreateTemp(cfg.assetDir, ".installer-initrd-*")
	if err != nil {
		return "", "", fmt.Errorf("create installer initrd temp file: %w", err)
	}
	tmpInitrdPath := tmpInitrd.Name()
	tmpInitrd.Close()
	os.Remove(tmpInitrdPath)
	defer os.Remove(tmpInitrdPath)

	args := []string{
		"-osirrox", "on",
		"-indev", iso,
		"-extract", spec.KernelPath, tmpKernelPath,
		"-extract", spec.InitrdPath, tmpInitrdPath,
	}
	if out, err := exec.Command("xorriso", args...).CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("extract installer kernel from %q: %w: %s", iso, err, strings.TrimSpace(string(out)))
	}
	for _, path := range []string{tmpKernelPath, tmpInitrdPath} {
		if err := os.Chmod(path, 0o644); err != nil {
			return "", "", fmt.Errorf("chmod installer kernel file: %w", err)
		}
	}
	if err := os.Rename(tmpKernelPath, kernel); err != nil {
		return "", "", fmt.Errorf("install installer kernel: %w", err)
	}
	if err := os.Rename(tmpInitrdPath, initrd); err != nil {
		return "", "", fmt.Errorf("install installer initrd: %w", err)
	}
	return kernel, initrd, nil
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

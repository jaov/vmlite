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

// prepareInstallerISO remasters the source ISO with installer.Spec.InjectFiles.
func prepareInstallerISO(cfg *Config, spec guestinstall.Spec) (string, error) {
	if len(spec.InjectFiles) == 0 {
		return "", nil
	}
	path, err := cfg.InstallerISOPath()
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
	if strings.TrimSpace(cfg.AssetDir) == "" {
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

	kernel := filepath.Join(cfg.AssetDir, "installer-vmlinuz")
	initrd := filepath.Join(cfg.AssetDir, "installer-initrd")
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
	if err := os.MkdirAll(cfg.AssetDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create installer asset directory: %w", err)
	}

	tmpKernel, err := os.CreateTemp(cfg.AssetDir, ".installer-vmlinuz-*")
	if err != nil {
		return "", "", fmt.Errorf("create installer kernel temp file: %w", err)
	}
	tmpKernelPath := tmpKernel.Name()
	tmpKernel.Close()
	os.Remove(tmpKernelPath)
	defer os.Remove(tmpKernelPath)

	tmpInitrd, err := os.CreateTemp(cfg.AssetDir, ".installer-initrd-*")
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
	spec, err := cfg.InstallerSpec()
	if err != nil {
		return "", err
	}
	if spec.Seed == nil {
		return "", nil
	}
	path, err := cfg.InstallerSeedPath()
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

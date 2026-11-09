package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// buildVirtInstallArgs translates cfg and libvirtURI into the virt-install
// argument list.
func buildVirtInstallArgs(cfg *Config, libvirtURI string) ([]string, error) {
	args := []string{
		"--connect", libvirtURI,
		"--name", cfg.Name,
		"--memory", strconv.Itoa(cfg.Memory),
		"--vcpus", strconv.Itoa(cfg.CPUs),
		"--disk", fmt.Sprintf("size=%d", cfg.DiskSize),
		"--os-variant", cfg.OSVariant,
		"--graphics", "none",
	}

	// Kernel arguments are only valid with a location/kernel install, so only
	// request the serial console when an install tree is configured. Without an
	// install source there is no install phase to wait for, so do not attach to
	// the guest console.
	if cfg.InstallURL != "" {
		args = append(args, "--location", cfg.InstallURL,
			"--extra-args", "console=ttyS0,115200n8")
	} else {
		args = append(args, "--install", "no_install=yes", "--noautoconsole")
	}

	switch libvirtURI {
	case systemURI:
		args = append(args, "--network", "network=default")
	case sessionURI:
		args = append(args, "--network", "user")
	default:
		return nil, fmt.Errorf("unsupported libvirt URI %q", libvirtURI)
	}

	for _, m := range cfg.Mounts {
		source, err := filepath.Abs(m.Source)
		if err != nil {
			return nil, fmt.Errorf("resolve mount source %q: %w", m.Source, err)
		}
		if _, err := os.Stat(source); err != nil {
			return nil, fmt.Errorf("mount source %q is not accessible: %w", m.Source, err)
		}
		args = append(args, "--filesystem",
			fmt.Sprintf("type=mount,source=%s,target=%s,accessmode=mapped", source, m.Target))
	}

	return args, nil
}

// RunVirtInstall provisions the VM described by cfg using virt-install.
// Output is streamed directly to the terminal.
func RunVirtInstall(cfg *Config, libvirtURI string) error {
	args, err := buildVirtInstallArgs(cfg, libvirtURI)
	if err != nil {
		return err
	}

	cmd := exec.Command("virt-install", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("virt-install failed: %w", err)
	}
	return nil
}

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// buildVirtInstallArgs translates cfg and libvirtURI into the virt-install
// argument list.
func buildVirtInstallArgs(cfg *Config, libvirtURI string) ([]string, error) {
	args := []string{
		"--connect", libvirtURI,
		"--name", cfg.Name,
		"--memory", strconv.Itoa(cfg.Memory),
		"--vcpus", strconv.Itoa(cfg.CPUs),
	}

	imageMode := strings.TrimSpace(cfg.Image) != ""
	if !imageMode {
		args = append(args, "--disk", fmt.Sprintf("size=%d", cfg.DiskSize))
	}

	args = append(args,
		"--os-variant", cfg.OSVariant,
		"--graphics", "none",
	)

	switch {
	case imageMode:
		overlay, err := cfg.overlayPath()
		if err != nil {
			return nil, err
		}
		args = append(args,
			"--import",
			"--disk", fmt.Sprintf("path=%s,format=qcow2", overlay),
			"--noautoconsole",
		)
		if strings.TrimSpace(cfg.CloudInit) != "" {
			seed, err := cfg.seedPath()
			if err != nil {
				return nil, err
			}
			args = append(args,
				"--disk", fmt.Sprintf("path=%s,device=cdrom,readonly=on", seed),
				"--sysinfo", "type=smbios,system.serial=ds=nocloud",
			)
		}
		args = append(args,
			"--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart",
		)

	case cfg.InstallURL != "":
		// Kernel arguments are only valid with a location/kernel install.
		args = append(args, "--location", cfg.InstallURL,
			"--extra-args", "console=ttyS0,115200n8")

	default:
		// Nothing to install: define and start the domain, then return.
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

	if strings.TrimSpace(cfg.Image) != "" {
		if _, err := prepareOverlay(cfg); err != nil {
			return err
		}
	}
	if _, err := prepareSeed(cfg); err != nil {
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

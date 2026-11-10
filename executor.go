package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	guestinstall "vmlite/internal/installer"
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

	osVariant := strings.TrimSpace(cfg.OSVariant)
	if osVariant == "" && cfg.Source.Type == sourceISO {
		// Installer media often has no osinfo entry (for example OpenBSD).
		// Ask virt-install to detect, but fall back to generic instead of
		// failing the install.
		osVariant = "detect=on,require=off"
	}
	if osVariant != "" {
		args = append(args, "--os-variant", osVariant)
	}
	spec, err := cfg.installerSpec()
	if err != nil {
		return nil, err
	}
	args = append(args, bootArgs(cfg.Firmware, spec.BootOrder)...)
	if cfg.hasInstaller() {
		// Installer disks are sparse qcow2 and often live on smaller tmpfs
		// pools; the fully-allocated capacity check is too pessimistic.
		args = append(args, "--check", "disk_size=off")
	}

	sourceArgs, err := buildSourceArgs(cfg, spec)
	if err != nil {
		return nil, err
	}
	args = append(args, sourceArgs...)

	graphicsArgs, err := buildGraphicsArgs(cfg)
	if err != nil {
		return nil, err
	}
	args = append(args, graphicsArgs...)

	if !cfg.Autoconsole {
		args = append(args, "--noautoconsole")
	}

	networkArgs, err := buildNetworkArgs(cfg, libvirtURI)
	if err != nil {
		return nil, err
	}
	args = append(args, networkArgs...)

	for _, d := range cfg.ExtraDisks {
		path, err := filepath.Abs(d.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve extra disk %q: %w", d.Path, err)
		}
		device := d.Device
		if device == "" {
			device = "disk"
		}
		format, err := extraDiskFormat(d)
		if err != nil {
			return nil, err
		}
		spec := fmt.Sprintf("path=%s,device=%s,format=%s", path, device, format)
		if d.ReadOnly {
			spec += ",readonly=on"
		}
		args = append(args, "--disk", spec)
	}

	for _, m := range cfg.Mounts {
		mountType := m.Type
		if mountType == "" {
			mountType = mountType9P
		}
		switch mountType {
		case mountTypeNone:
			continue
		case mountTypeVirtioFS:
			return nil, fmt.Errorf("mount type %q is not implemented yet", mountTypeVirtioFS)
		}

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

func buildSourceArgs(cfg *Config, spec guestinstall.Spec) ([]string, error) {
	switch cfg.Source.Type {
	case sourceNone:
		diskArgs, err := primaryDiskArgs(cfg)
		if err != nil {
			return nil, err
		}
		return append(diskArgs, "--install", "no_install=yes"), nil

	case sourceCloudImage:
		overlay, err := cfg.overlayPath()
		if err != nil {
			return nil, err
		}
		args := []string{
			"--import",
			"--disk", fmt.Sprintf("path=%s,format=qcow2", overlay),
		}
		if cfg.needsSeed() {
			seed, err := cfg.seedPath()
			if err != nil {
				return nil, err
			}
			args = append(args,
				"--disk", fmt.Sprintf("path=%s,format=raw,readonly=on", seed),
				"--sysinfo", "type=smbios,system.serial=ds=nocloud",
			)
		}
		args = append(args, "--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart")
		return args, nil

	case sourceInstallTree:
		diskArgs, err := primaryDiskArgs(cfg)
		if err != nil {
			return nil, err
		}
		args := append(diskArgs, "--location", cfg.Source.URL)
		if strings.TrimSpace(cfg.Source.ExtraArgs) != "" {
			args = append(args, "--extra-args", cfg.Source.ExtraArgs)
		}
		return args, nil

	case sourceISO:
		diskArgs, err := primaryDiskArgs(cfg)
		if err != nil {
			return nil, err
		}
		if spec.Type != "" {
			diskArgs, err = primaryDiskArgsBus(cfg, "virtio")
			if err != nil {
				return nil, err
			}
		}
		iso, err := cfg.installerISOPath()
		if err != nil {
			return nil, err
		}
		args := diskArgs
		if len(spec.KernelArgs) > 0 {
			args = append(args, "--location", iso, "--extra-args", strings.Join(spec.KernelArgs, " "))
		} else {
			args = append(args, "--cdrom", iso)
		}
		if spec.Seed != nil {
			seedPath, err := cfg.installerSeedPath()
			if err != nil {
				return nil, err
			}
			device := spec.SeedDevice
			if device == "" {
				device = "disk"
			}
			diskSpec := fmt.Sprintf("path=%s,format=raw,readonly=on,device=%s", seedPath, device)
			if device == "disk" {
				diskSpec += ",bus=virtio"
			}
			args = append(args,
				"--disk", diskSpec,
				"--sysinfo", "type=smbios,system.serial=ds=nocloud",
			)
		}
		return args, nil

	case sourceDisk:
		format, err := cfg.sourceDiskFormat()
		if err != nil {
			return nil, err
		}
		path, err := filepath.Abs(cfg.Source.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve disk %q: %w", cfg.Source.Path, err)
		}
		return []string{
			"--import",
			"--disk", fmt.Sprintf("path=%s,format=%s", path, format),
			"--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart",
		}, nil

	case sourcePXE:
		diskArgs, err := primaryDiskArgs(cfg)
		if err != nil {
			return nil, err
		}
		return append(diskArgs, "--pxe"), nil

	default:
		return nil, fmt.Errorf("unsupported source.type %q", cfg.Source.Type)
	}
}

func primaryDiskArgs(cfg *Config) ([]string, error) {
	if strings.TrimSpace(cfg.Disk.Path) != "" {
		path, err := filepath.Abs(cfg.Disk.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve disk path %q: %w", cfg.Disk.Path, err)
		}
		return []string{"--disk", fmt.Sprintf("path=%s,size=%d,format=%s", path, cfg.Disk.Size, cfg.newDiskFormat())}, nil
	}
	return []string{"--disk", fmt.Sprintf("size=%d", cfg.Disk.Size)}, nil
}

func primaryDiskArgsBus(cfg *Config, bus string) ([]string, error) {
	args, err := primaryDiskArgs(cfg)
	if err != nil || bus == "" {
		return args, err
	}
	if len(args) == 2 && args[0] == "--disk" {
		args[1] += ",bus=" + bus
	}
	return args, nil
}

// bootArgs composes firmware and boot order into one --boot argument. A boot
// order is only needed when a source must boot installer media and then switch
// to the installed disk.
func bootArgs(firmware string, order []string) []string {
	if firmware == firmwareUEFI {
		if len(order) == 0 {
			return []string{"--boot", "uefi"}
		}
		return []string{"--boot", "uefi," + strings.Join(order, ",")}
	}
	if len(order) == 0 {
		return nil
	}
	return []string{"--boot", strings.Join(order, ",")}
}

func buildGraphicsArgs(cfg *Config) ([]string, error) {
	graphicsType := cfg.Graphics.Type
	if graphicsType == "" {
		graphicsType = graphicsNone
	}
	listen := cfg.Graphics.Listen
	if listen == "" {
		listen = "127.0.0.1"
	}

	switch graphicsType {
	case graphicsNone:
		return []string{"--graphics", "none"}, nil
	case graphicsVNC:
		return []string{"--graphics", "vnc,listen=" + listen}, nil
	case graphicsSPICE:
		return []string{"--graphics", "spice,listen=" + listen}, nil
	default:
		return nil, fmt.Errorf("unsupported graphics.type %q", graphicsType)
	}
}

func buildNetworkArgs(cfg *Config, libvirtURI string) ([]string, error) {
	mode := cfg.Network.Mode
	if mode == "" {
		mode = networkModeAuto
	}

	var spec string
	switch mode {
	case networkModeAuto:
		switch libvirtURI {
		case systemURI:
			spec = "network=default"
		case sessionURI:
			spec = "user"
		default:
			return nil, fmt.Errorf("unsupported libvirt URI %q", libvirtURI)
		}
	case networkModeDefault:
		spec = "network=default"
	case networkModeUser:
		spec = "user"
	case networkModeBridge:
		spec = "bridge=" + cfg.Network.Bridge
	default:
		return nil, fmt.Errorf("unsupported network.mode %q", mode)
	}

	if strings.TrimSpace(cfg.Network.Model) != "" {
		spec += ",model=" + cfg.Network.Model
	}
	return []string{"--network", spec}, nil
}

// RunVirtInstall provisions the VM described by cfg using virt-install.
// Output is streamed directly to the terminal.
func RunVirtInstall(cfg *Config, libvirtURI string) error {
	args, err := buildVirtInstallArgs(cfg, libvirtURI)
	if err != nil {
		return err
	}

	if cfg.Source.Type == sourceCloudImage {
		if _, err := prepareOverlay(cfg); err != nil {
			return err
		}
	}
	if _, err := prepareSeed(cfg); err != nil {
		return err
	}
	spec, err := cfg.installerSpec()
	if err != nil {
		return err
	}
	if _, err := prepareInstallerISO(cfg, spec); err != nil {
		return err
	}
	if _, err := prepareInstallerSeed(cfg); err != nil {
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

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

	if strings.TrimSpace(cfg.OSVariant) != "" {
		args = append(args, "--os-variant", cfg.OSVariant)
	}
	if cfg.Firmware == firmwareUEFI {
		args = append(args, "--boot", "uefi")
	}

	sourceArgs, err := buildSourceArgs(cfg)
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
		spec := fmt.Sprintf("path=%s,device=%s,format=%s", path, device, extraDiskFormat(d))
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

func buildSourceArgs(cfg *Config) ([]string, error) {
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
		if strings.TrimSpace(cfg.Source.CloudInit) != "" {
			seed, err := cfg.seedPath()
			if err != nil {
				return nil, err
			}
			args = append(args,
				"--disk", fmt.Sprintf("path=%s,device=cdrom,readonly=on", seed),
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
		iso, err := filepath.Abs(cfg.Source.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve iso %q: %w", cfg.Source.Path, err)
		}
		return append(diskArgs, "--cdrom", iso), nil

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

	cmd := exec.Command("virt-install", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("virt-install failed: %w", err)
	}
	return nil
}

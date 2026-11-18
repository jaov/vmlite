package vm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"vmlite/internal/config"
	"vmlite/internal/host"
	guestinstall "vmlite/internal/installer"
	"vmlite/internal/libvirt"
	"vmlite/internal/media"
)

// buildVirtInstallArgs translates cfg and libvirtURI into the virt-install
// argument list.
func BuildVirtInstallArgs(cfg *config.Config, libvirtURI string) ([]string, error) {
	args := []string{
		"--connect", libvirtURI,
		"--name", cfg.Name,
		"--memory", strconv.Itoa(cfg.Memory),
		"--vcpus", strconv.Itoa(cfg.CPUs),
	}

	osVariant := strings.TrimSpace(cfg.OSVariant)
	if osVariant == "" && cfg.Source.Type == config.SourceISO {
		// Installer media often has no osinfo entry (for example OpenBSD).
		// Ask virt-install to detect, but fall back to generic instead of
		// failing the install.
		osVariant = "detect=on,require=off"
	}
	if osVariant != "" {
		args = append(args, "--os-variant", osVariant)
	}
	spec, err := cfg.InstallerSpec()
	if err != nil {
		return nil, err
	}
	if spec.KernelBoot {
		if cfg.Firmware == config.FirmwareUEFI {
			return nil, fmt.Errorf("installer %q uses direct kernel boot, which currently requires firmware %q", spec.Type, config.FirmwareBIOS)
		}
	} else {
		args = append(args, BootArgs(cfg.Firmware, spec.BootOrder, cfg.SecureBoot)...)
	}
	if cfg.TPM != nil {
		args = append(args, "--tpm", fmt.Sprintf("backend.type=%s,backend.version=%s", cfg.TPMBackend(), cfg.TPMVersion()))
	}
	if cfg.HasInstaller() {
		// Installer disks are sparse qcow2 and often live on smaller tmpfs
		// pools; the fully-allocated capacity check is too pessimistic.
		args = append(args, "--check", "disk_size=off")
	}

	sourceArgs, err := BuildSourceArgs(cfg, spec)
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
		format, err := media.ExtraDiskFormat(d)
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
			mountType = config.MountType9P
		}
		switch mountType {
		case config.MountTypeNone:
			continue
		case config.MountTypeVirtioFS:
			return nil, fmt.Errorf("mount type %q is not implemented yet", config.MountTypeVirtioFS)
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

func BuildSourceArgs(cfg *config.Config, spec guestinstall.Spec) ([]string, error) {
	switch cfg.Source.Type {
	case config.SourceNone:
		diskArgs, err := primaryDiskArgs(cfg)
		if err != nil {
			return nil, err
		}
		return append(diskArgs, "--install", "no_install=yes"), nil

	case config.SourceCloudImage:
		overlay, err := cfg.OverlayPath()
		if err != nil {
			return nil, err
		}
		diskSpec := fmt.Sprintf("path=%s,format=qcow2", overlay)
		if bus := cfg.DiskBus(); bus != "" {
			diskSpec += ",bus=" + bus
		}
		args := []string{
			"--import",
			"--disk", diskSpec,
		}
		if cfg.NeedsSeed() {
			seed, err := cfg.SeedPath()
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

	case config.SourceInstallTree:
		diskArgs, err := primaryDiskArgs(cfg)
		if err != nil {
			return nil, err
		}
		args := append(diskArgs, "--location", cfg.Source.URL)
		if strings.TrimSpace(cfg.Source.ExtraArgs) != "" {
			args = append(args, "--extra-args", cfg.Source.ExtraArgs)
		}
		return args, nil

	case config.SourceISO:
		diskArgs, err := primaryDiskArgs(cfg)
		if err != nil {
			return nil, err
		}
		if spec.Type != "" {
			defaultBus := spec.DiskBus
			if defaultBus == "" {
				defaultBus = "virtio"
			}
			diskArgs, err = primaryDiskArgsBus(cfg, defaultBus)
			if err != nil {
				return nil, err
			}
		}
		iso, err := cfg.InstallerISOPath()
		if err != nil {
			return nil, err
		}
		args := diskArgs
		if spec.KernelBoot {
			kernel, initrd, err := media.PrepareInstallerKernel(cfg, spec)
			if err != nil {
				return nil, err
			}
			boot := fmt.Sprintf("kernel=%s,initrd=%s", kernel, initrd)
			if len(spec.KernelArgs) > 0 {
				boot += ",kernel_args=" + strings.Join(spec.KernelArgs, " ")
			}
			args = append(args, "--cdrom", iso, "--boot", boot)
		} else if len(spec.KernelArgs) > 0 {
			args = append(args, "--location", iso, "--extra-args", strings.Join(spec.KernelArgs, " "))
		} else {
			args = append(args, "--cdrom", iso)
		}
		if spec.Seed != nil {
			seedPath, err := cfg.InstallerSeedPath()
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
		if spec.Completion == guestinstall.CompletionSSH {
			args = append(args, "--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart")
		}
		return args, nil

	case config.SourceDisk:
		format, err := cfg.SourceDiskFormat()
		if err != nil {
			return nil, err
		}
		path, err := filepath.Abs(cfg.Source.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve disk %q: %w", cfg.Source.Path, err)
		}
		diskSpec := fmt.Sprintf("path=%s,format=%s", path, format)
		if bus := cfg.DiskBus(); bus != "" {
			diskSpec += ",bus=" + bus
		}
		return []string{
			"--import",
			"--disk", diskSpec,
			"--events", "on_reboot=restart,on_poweroff=destroy,on_crash=restart",
		}, nil

	case config.SourcePXE:
		diskArgs, err := primaryDiskArgs(cfg)
		if err != nil {
			return nil, err
		}
		return append(diskArgs, "--pxe"), nil

	default:
		return nil, fmt.Errorf("unsupported source.type %q", cfg.Source.Type)
	}
}

func primaryDiskArgs(cfg *config.Config) ([]string, error) {
	bus := cfg.DiskBus()
	if strings.TrimSpace(cfg.Disk.Path) != "" {
		path, err := filepath.Abs(cfg.Disk.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve disk path %q: %w", cfg.Disk.Path, err)
		}
		spec := fmt.Sprintf("path=%s,size=%d,format=%s", path, cfg.Disk.Size, cfg.NewDiskFormat())
		if bus != "" {
			spec += ",bus=" + bus
		}
		return []string{"--disk", spec}, nil
	}
	spec := fmt.Sprintf("size=%d", cfg.Disk.Size)
	if bus != "" {
		spec += ",bus=" + bus
	}
	return []string{"--disk", spec}, nil
}

func primaryDiskArgsBus(cfg *config.Config, defaultBus string) ([]string, error) {
	args, err := primaryDiskArgs(cfg)
	if err != nil {
		return args, err
	}
	if cfg.DiskBus() != "" || defaultBus == "" {
		return args, nil
	}
	if len(args) == 2 && args[0] == "--disk" {
		args[1] += ",bus=" + defaultBus
	}
	return args, nil
}

// bootArgs composes firmware and boot order into one --boot argument. A boot
// order is only needed when a source must boot installer media and then switch
// to the installed disk.
func BootArgs(firmware string, order []string, secureBoot bool) []string {
	if firmware == config.FirmwareUEFI {
		parts := []string{"uefi"}
		if secureBoot {
			parts = append(parts, "loader.secure=on")
		}
		parts = append(parts, order...)
		return []string{"--boot", strings.Join(parts, ",")}
	}
	if len(order) == 0 {
		return nil
	}
	return []string{"--boot", strings.Join(order, ",")}
}

func buildGraphicsArgs(cfg *config.Config) ([]string, error) {
	graphicsType := cfg.Graphics.Type
	if graphicsType == "" {
		graphicsType = config.GraphicsNone
	}
	listen := cfg.Graphics.Listen
	if listen == "" {
		listen = "127.0.0.1"
	}

	switch graphicsType {
	case config.GraphicsNone:
		return []string{"--graphics", "none"}, nil
	case config.GraphicsVNC:
		return []string{"--graphics", "vnc,listen=" + listen}, nil
	case config.GraphicsSPICE:
		return []string{"--graphics", "spice,listen=" + listen}, nil
	default:
		return nil, fmt.Errorf("unsupported graphics.type %q", graphicsType)
	}
}

func buildNetworkArgs(cfg *config.Config, libvirtURI string) ([]string, error) {
	mode := cfg.Network.Mode
	if mode == "" {
		mode = config.NetworkModeAuto
	}

	var spec string
	switch mode {
	case config.NetworkModeAuto:
		switch libvirtURI {
		case host.SystemURI:
			spec = "network=default"
		case host.SessionURI:
			spec = "user"
		default:
			return nil, fmt.Errorf("unsupported libvirt URI %q", libvirtURI)
		}
	case config.NetworkModeDefault:
		spec = "network=default"
	case config.NetworkModeUser:
		spec = "user"
	case config.NetworkModeBridge:
		spec = "bridge=" + cfg.Network.Bridge
	default:
		return nil, fmt.Errorf("unsupported network.mode %q", mode)
	}

	if strings.TrimSpace(cfg.Network.Model) != "" {
		spec += ",model=" + cfg.Network.Model
	}
	if strings.TrimSpace(cfg.NetworkMAC) != "" {
		spec += ",mac=" + cfg.NetworkMAC
	}
	return []string{"--network", spec}, nil
}

// RunVirtInstall provisions the VM described by cfg using virt-install.
// Output is streamed directly to the terminal.
func RunVirtInstall(cfg *config.Config, libvirtURI string) error {
	args, err := BuildVirtInstallArgs(cfg, libvirtURI)
	if err != nil {
		return err
	}

	if cfg.Source.Type == config.SourceCloudImage {
		if _, err := media.PrepareOverlay(cfg); err != nil {
			return err
		}
	}
	if _, err := media.PrepareSeed(cfg); err != nil {
		return err
	}
	spec, err := cfg.InstallerSpec()
	if err != nil {
		return err
	}
	if _, err := media.PrepareInstallerISO(cfg, spec); err != nil {
		return err
	}
	if _, err := media.PrepareInstallerSeed(cfg); err != nil {
		return err
	}

	cmd := exec.Command("virt-install", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("virt-install failed: %w", err)
	}
	if err := ConfigureInstallerLifecycle(cfg, libvirtURI, spec); err != nil {
		return err
	}
	return nil
}

func ConfigureInstallerLifecycle(cfg *config.Config, libvirtURI string, spec guestinstall.Spec) error {
	if spec.Completion != guestinstall.CompletionSSH {
		return nil
	}
	if out, err := libvirt.Run(libvirtURI, "set-lifecycle-action", cfg.Name, "reboot", "restart", "--live", "--config"); err != nil {
		return fmt.Errorf("configure installer reboot lifecycle for %q: %w: %s", cfg.Name, err, strings.TrimSpace(out))
	}
	return nil
}

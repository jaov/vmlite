package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	sourceNone        = "none"
	sourceCloudImage  = "cloud_image"
	sourceInstallTree = "install_tree"
	sourceISO         = "iso"
	sourceDisk        = "disk"
	sourcePXE         = "pxe"

	firmwareBIOS = "bios"
	firmwareUEFI = "uefi"

	mountType9P       = "9p"
	mountTypeNone     = "none"
	mountTypeVirtioFS = "virtiofs"

	networkModeAuto    = "auto"
	networkModeDefault = "default"
	networkModeUser    = "user"
	networkModeBridge  = "bridge"

	graphicsNone  = "none"
	graphicsVNC   = "vnc"
	graphicsSPICE = "spice"

	diskFormatAuto  = "auto"
	diskFormatQCOW2 = "qcow2"
	diskFormatRaw   = "raw"
)

// Config describes the virtual machine that vmlite creates.
type Config struct {
	Name        string      `json:"name"`
	Memory      int         `json:"memory"` // in MiB
	CPUs        int         `json:"cpus"`
	OSVariant   string      `json:"os_variant,omitempty"`
	Firmware    string      `json:"firmware,omitempty"`
	Source      Source      `json:"source"`
	Disk        Disk        `json:"disk"`
	Mounts      []Mount     `json:"mounts,omitempty"`
	Network     Network     `json:"network,omitempty"`
	Graphics    Graphics    `json:"graphics,omitempty"`
	Autoconsole bool        `json:"autoconsole,omitempty"`
	ExtraDisks  []ExtraDisk `json:"extra_disks,omitempty"`
}

// Source selects what the VM boots from.
type Source struct {
	Type      string `json:"type"`
	Path      string `json:"path,omitempty"`
	URL       string `json:"url,omitempty"`
	CloudInit string `json:"cloud_init,omitempty"`
	ExtraArgs string `json:"extra_args,omitempty"`
}

// Disk describes the VM's primary disk.
type Disk struct {
	Size   int    `json:"size"`             // in GiB
	Path   string `json:"path,omitempty"`   // optional explicit path
	Format string `json:"format,omitempty"` // auto, qcow2, raw
}

// Mount is a host directory exposed to the guest.
type Mount struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type,omitempty"` // 9p, virtiofs, none
}

// Network controls the guest's primary interface.
type Network struct {
	Mode   string `json:"mode,omitempty"` // auto, default, user, bridge
	Bridge string `json:"bridge,omitempty"`
	Model  string `json:"model,omitempty"`
}

// Graphics controls the guest display.
type Graphics struct {
	Type   string `json:"type,omitempty"` // none, vnc, spice
	Listen string `json:"listen,omitempty"`
}

// ExtraDisk attaches additional media, such as installer config discs.
type ExtraDisk struct {
	Path     string `json:"path"`
	Device   string `json:"device,omitempty"` // disk, cdrom
	Format   string `json:"format,omitempty"` // auto, qcow2, raw
	ReadOnly bool   `json:"readonly,omitempty"`
}

// LoadConfig reads and validates a JSON configuration file.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	dec := json.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse config %q: unexpected trailing data", path)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %q: %w", path, err)
	}

	return &cfg, nil
}

// Validate checks that required values are present and sane.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if c.Memory <= 0 {
		return fmt.Errorf("memory must be greater than zero")
	}
	if c.CPUs <= 0 {
		return fmt.Errorf("cpus must be greater than zero")
	}

	switch c.Firmware {
	case "", firmwareBIOS, firmwareUEFI:
	default:
		return fmt.Errorf("firmware must be %q or %q", firmwareBIOS, firmwareUEFI)
	}

	if err := c.validateSource(); err != nil {
		return err
	}

	switch c.Disk.Format {
	case "", diskFormatAuto, diskFormatQCOW2, diskFormatRaw:
	default:
		return fmt.Errorf("disk.format must be %q, %q, or %q", diskFormatAuto, diskFormatQCOW2, diskFormatRaw)
	}

	for i, m := range c.Mounts {
		if strings.TrimSpace(m.Source) == "" {
			return fmt.Errorf("mounts[%d].source is required", i)
		}
		if strings.TrimSpace(m.Target) == "" {
			return fmt.Errorf("mounts[%d].target is required", i)
		}
		switch m.Type {
		case "", mountType9P, mountTypeNone, mountTypeVirtioFS:
		default:
			return fmt.Errorf("mounts[%d].type must be %q, %q, or %q", i, mountType9P, mountTypeVirtioFS, mountTypeNone)
		}
	}

	switch c.Network.Mode {
	case "", networkModeAuto, networkModeDefault, networkModeUser, networkModeBridge:
	default:
		return fmt.Errorf("network.mode must be %q, %q, %q, or %q", networkModeAuto, networkModeDefault, networkModeUser, networkModeBridge)
	}
	if c.Network.Mode == networkModeBridge && strings.TrimSpace(c.Network.Bridge) == "" {
		return fmt.Errorf("network.bridge is required when network.mode is %q", networkModeBridge)
	}

	switch c.Graphics.Type {
	case "", graphicsNone, graphicsVNC, graphicsSPICE:
	default:
		return fmt.Errorf("graphics.type must be %q, %q, or %q", graphicsNone, graphicsVNC, graphicsSPICE)
	}

	for i, d := range c.ExtraDisks {
		if strings.TrimSpace(d.Path) == "" {
			return fmt.Errorf("extra_disks[%d].path is required", i)
		}
		switch d.Device {
		case "", "disk", "cdrom":
		default:
			return fmt.Errorf("extra_disks[%d].device must be %q or %q", i, "disk", "cdrom")
		}
		switch d.Format {
		case "", diskFormatAuto, diskFormatQCOW2, diskFormatRaw:
		default:
			return fmt.Errorf("extra_disks[%d].format must be %q, %q, or %q", i, diskFormatAuto, diskFormatQCOW2, diskFormatRaw)
		}
	}

	return nil
}

func (c *Config) validateSource() error {
	src := c.Source
	switch src.Type {
	case sourceNone:
		if src.Path != "" || src.URL != "" || src.CloudInit != "" || src.ExtraArgs != "" {
			return fmt.Errorf("source.type %q does not accept path, url, cloud_init, or extra_args", sourceNone)
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceCloudImage:
		if strings.TrimSpace(src.Path) == "" {
			return fmt.Errorf("source.path is required for source.type %q", sourceCloudImage)
		}
		if src.URL != "" || src.ExtraArgs != "" {
			return fmt.Errorf("source.url and source.extra_args are not valid for source.type %q", sourceCloudImage)
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceInstallTree:
		if strings.TrimSpace(src.URL) == "" {
			return fmt.Errorf("source.url is required for source.type %q", sourceInstallTree)
		}
		if src.Path != "" || src.CloudInit != "" {
			return fmt.Errorf("source.path and source.cloud_init are not valid for source.type %q", sourceInstallTree)
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceISO:
		if strings.TrimSpace(src.Path) == "" {
			return fmt.Errorf("source.path is required for source.type %q", sourceISO)
		}
		if src.URL != "" || src.CloudInit != "" || src.ExtraArgs != "" {
			return fmt.Errorf("source.url, source.cloud_init, and source.extra_args are not valid for source.type %q", sourceISO)
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceDisk:
		if strings.TrimSpace(src.Path) == "" {
			return fmt.Errorf("source.path is required for source.type %q", sourceDisk)
		}
		if src.URL != "" || src.CloudInit != "" || src.ExtraArgs != "" {
			return fmt.Errorf("source.url, source.cloud_init, and source.extra_args are not valid for source.type %q", sourceDisk)
		}
		if c.Disk.Path != "" {
			return fmt.Errorf("disk.path is not used with source.type %q", sourceDisk)
		}

	case sourcePXE:
		if src.Path != "" || src.URL != "" || src.CloudInit != "" || src.ExtraArgs != "" {
			return fmt.Errorf("source.type %q does not accept path, url, cloud_init, or extra_args", sourcePXE)
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	default:
		return fmt.Errorf("source.type must be %q, %q, %q, %q, %q, or %q",
			sourceNone, sourceCloudImage, sourceInstallTree, sourceISO, sourceDisk, sourcePXE)
	}

	return nil
}

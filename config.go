package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"vmlite/internal/installer"
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
	Name         string        `json:"name"`
	Memory       int           `json:"memory"` // in MiB
	CPUs         int           `json:"cpus"`
	OSVariant    string        `json:"os_variant,omitempty"`
	Firmware     string        `json:"firmware,omitempty"`
	Source       Source        `json:"source"`
	Disk         Disk          `json:"disk"`
	Mounts       []Mount       `json:"mounts,omitempty"`
	Network      Network       `json:"network,omitempty"`
	Graphics     Graphics      `json:"graphics,omitempty"`
	Autoconsole  bool          `json:"autoconsole,omitempty"`
	ExtraDisks   []ExtraDisk   `json:"extra_disks,omitempty"`
	SSH          SSHConfig     `json:"ssh,omitempty"`
	Provisioners []Provisioner `json:"provisioners,omitempty"`

	sshPublicKey    string
	sshKeyGenerated bool
	configDir       string
	assetDir        string
}

// SSHConfig controls the guest account used for shell provisioners.
type SSHConfig struct {
	User string `json:"user,omitempty"`
	Key  string `json:"key,omitempty"`
}

// Provisioner is a post-boot shell action.
type Provisioner struct {
	Type   string `json:"type,omitempty"`
	Path   string `json:"path,omitempty"`
	Inline string `json:"inline,omitempty"`
	Run    string `json:"run,omitempty"` // once, always
}

func (c *Config) sshUser() string {
	if strings.TrimSpace(c.SSH.User) != "" {
		return c.SSH.User
	}
	return "vmlite"
}

func (c *Config) needsSSH() bool {
	return len(c.Provisioners) > 0 || strings.TrimSpace(c.SSH.User) != "" || strings.TrimSpace(c.SSH.Key) != ""
}

func (c *Config) needsSeed() bool {
	return c.Source.Type == sourceCloudImage && (strings.TrimSpace(c.Source.CloudInit) != "" || c.needsSSH())
}

func (c *Config) hasInstaller() bool {
	return c.Source.Type == sourceISO && c.Source.Installer != nil
}

func (c *Config) canGenerateSSHKey() bool {
	return c.Source.Type == sourceCloudImage || c.hasInstaller()
}

// Source selects what the VM boots from.
type Source struct {
	Type      string     `json:"type"`
	Path      string     `json:"path,omitempty"`
	URL       string     `json:"url,omitempty"`
	SHA256    string     `json:"sha256,omitempty"`
	Cache     string     `json:"cache,omitempty"`
	CloudInit string     `json:"cloud_init,omitempty"`
	ExtraArgs string     `json:"extra_args,omitempty"`
	Installer *Installer `json:"installer,omitempty"`
}

// Installer selects unattended installation for ISO sources.
type Installer struct {
	Type   string `json:"type"`
	Config string `json:"config,omitempty"`
	Inline string `json:"inline,omitempty"`
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
	if abs, err := filepath.Abs(path); err == nil {
		cfg.configDir = filepath.Dir(abs)
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

	if c.needsSSH() {
		switch c.Source.Type {
		case sourceCloudImage:
		case sourceISO:
			if c.Source.Installer == nil && strings.TrimSpace(c.SSH.Key) == "" {
				return fmt.Errorf("ssh.key is required for manual ISO provisioning")
			}
		case sourceDisk:
			if strings.TrimSpace(c.SSH.Key) == "" {
				return fmt.Errorf("ssh.key is required for disk provisioning")
			}
		default:
			return fmt.Errorf("ssh/provisioners are not supported for source.type %q", c.Source.Type)
		}
	}
	if len(c.Provisioners) > 0 {
		if c.Network.Mode == networkModeUser {
			return fmt.Errorf("provisioners require a guest-visible network; network.mode %q is not supported", networkModeUser)
		}
	}
	for i, p := range c.Provisioners {
		switch p.Type {
		case "", "shell":
		default:
			return fmt.Errorf("provisioners[%d].type must be %q", i, "shell")
		}
		hasPath := strings.TrimSpace(p.Path) != ""
		hasInline := p.Inline != ""
		if hasPath == hasInline {
			return fmt.Errorf("provisioners[%d] requires exactly one of path or inline", i)
		}
		switch p.Run {
		case "", "once", "always":
		default:
			return fmt.Errorf("provisioners[%d].run must be %q or %q", i, "once", "always")
		}
	}

	return nil
}

func validSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func (c *Config) validateSource() error {
	src := c.Source
	switch src.Type {
	case sourceNone:
		if src.Path != "" || src.URL != "" || src.CloudInit != "" || src.ExtraArgs != "" || src.SHA256 != "" || src.Cache != "" {
			return fmt.Errorf("source.type %q does not accept path, url, cloud_init, extra_args, sha256, or cache", sourceNone)
		}
		if src.Installer != nil {
			return fmt.Errorf("source.installer requires source.type %q", sourceISO)
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceCloudImage:
		hasPath := strings.TrimSpace(src.Path) != ""
		hasURL := strings.TrimSpace(src.URL) != ""
		if hasPath == hasURL {
			return fmt.Errorf("source.type %q requires exactly one of source.path or source.url", sourceCloudImage)
		}
		if src.ExtraArgs != "" {
			return fmt.Errorf("source.extra_args is not valid for source.type %q", sourceCloudImage)
		}
		if src.Installer != nil {
			return fmt.Errorf("source.installer requires source.type %q", sourceISO)
		}
		if hasURL {
			if !validSHA256(src.SHA256) {
				return fmt.Errorf("source.sha256 must be a 64-character hex digest for URL images")
			}
		} else if src.SHA256 != "" && !validSHA256(src.SHA256) {
			return fmt.Errorf("source.sha256 must be a 64-character hex digest")
		}
		if src.Cache != "" && !hasURL {
			return fmt.Errorf("source.cache requires source.url")
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceInstallTree:
		if strings.TrimSpace(src.URL) == "" {
			return fmt.Errorf("source.url is required for source.type %q", sourceInstallTree)
		}
		if src.Path != "" || src.CloudInit != "" || src.SHA256 != "" || src.Cache != "" {
			return fmt.Errorf("source.path, source.cloud_init, source.sha256, and source.cache are not valid for source.type %q", sourceInstallTree)
		}
		if src.Installer != nil {
			return fmt.Errorf("source.installer requires source.type %q", sourceISO)
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceISO:
		if strings.TrimSpace(src.Path) == "" {
			return fmt.Errorf("source.path is required for source.type %q", sourceISO)
		}
		if src.URL != "" || src.CloudInit != "" || src.ExtraArgs != "" || src.SHA256 != "" || src.Cache != "" {
			return fmt.Errorf("source.url, source.cloud_init, source.extra_args, source.sha256, and source.cache are not valid for source.type %q", sourceISO)
		}
		if err := validateInstaller(src.Installer); err != nil {
			return err
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceDisk:
		if strings.TrimSpace(src.Path) == "" {
			return fmt.Errorf("source.path is required for source.type %q", sourceDisk)
		}
		if src.URL != "" || src.CloudInit != "" || src.ExtraArgs != "" || src.SHA256 != "" || src.Cache != "" {
			return fmt.Errorf("source.url, source.cloud_init, source.extra_args, source.sha256, and source.cache are not valid for source.type %q", sourceDisk)
		}
		if src.Installer != nil {
			return fmt.Errorf("source.installer requires source.type %q", sourceISO)
		}
		if c.Disk.Path != "" {
			return fmt.Errorf("disk.path is not used with source.type %q", sourceDisk)
		}

	case sourcePXE:
		if src.Path != "" || src.URL != "" || src.CloudInit != "" || src.ExtraArgs != "" || src.SHA256 != "" || src.Cache != "" {
			return fmt.Errorf("source.type %q does not accept path, url, cloud_init, extra_args, sha256, or cache", sourcePXE)
		}
		if src.Installer != nil {
			return fmt.Errorf("source.installer requires source.type %q", sourceISO)
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

func validateInstaller(inst *Installer) error {
	if inst == nil {
		return nil
	}
	if strings.TrimSpace(inst.Type) == "" {
		return fmt.Errorf("source.installer.type is required")
	}
	if _, err := installer.Lookup(inst.Type); err != nil {
		return fmt.Errorf("source.installer: %w", err)
	}
	hasConfig := strings.TrimSpace(inst.Config) != ""
	hasInline := inst.Inline != ""
	if hasConfig && hasInline {
		return fmt.Errorf("source.installer requires at most one of config or inline")
	}
	return nil
}

package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"vmlite/internal/installer"
	"vmlite/internal/state"
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

	diskBusAuto   = "auto"
	diskBusVirtio = "virtio"
	diskBusSATA   = "sata"
	diskBusSCSI   = "scsi"

	tpmBackendEmulator = "emulator"
	tpmVersion12       = "1.2"
	tpmVersion20       = "2.0"
)

// Config describes the virtual machine that vmlite creates.
type Config struct {
	Name         string        `json:"name"`
	StateDir     string        `json:"state_dir,omitempty"`
	Autostart    bool          `json:"autostart,omitempty"`
	Memory       int           `json:"memory"` // in MiB
	CPUs         int           `json:"cpus"`
	OSVariant    string        `json:"os_variant,omitempty"`
	Firmware     string        `json:"firmware,omitempty"`
	SecureBoot   bool          `json:"secure_boot,omitempty"`
	TPM          *TPM          `json:"tpm,omitempty"`
	Source       Source        `json:"source"`
	Disk         Disk          `json:"disk"`
	Mounts       []Mount       `json:"mounts,omitempty"`
	Network      Network       `json:"network,omitempty"`
	Graphics     Graphics      `json:"graphics,omitempty"`
	Autoconsole  bool          `json:"autoconsole,omitempty"`
	ExtraDisks   []ExtraDisk   `json:"extra_disks,omitempty"`
	SSH          SSHConfig     `json:"ssh,omitempty"`
	Provisioners []Provisioner `json:"provisioners,omitempty"`

	SSHPublicKey    string `json:"-"`
	SSHKeyGenerated bool   `json:"-"`
	WindowsPassword string `json:"-"`
	ConfigDir       string `json:"-"`
	AssetDir        string `json:"-"`
}

// SSHConfig controls the guest account used for shell provisioners.
type SSHConfig struct {
	User string `json:"user,omitempty"`
	Key  string `json:"key,omitempty"`
}

// Provisioner is a post-boot script and optional file uploads.
type Provisioner struct {
	Type    string            `json:"type,omitempty"`
	Path    string            `json:"path,omitempty"`
	Inline  string            `json:"inline,omitempty"`
	Run     string            `json:"run,omitempty"` // once, always
	Uploads []Upload          `json:"uploads,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Upload copies a host file into the guest before a provisioner runs.
type Upload struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func (c *Config) sshUser() string {
	if strings.TrimSpace(c.SSH.User) != "" {
		return c.SSH.User
	}
	if c.Source.Installer != nil {
		if impl, err := installer.Lookup(c.Source.Installer.Type); err == nil {
			if d, ok := impl.(interface{ DefaultSSHUser() string }); ok {
				if user := strings.TrimSpace(d.DefaultSSHUser()); user != "" {
					return user
				}
			}
		}
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
	Type    string            `json:"type"`
	Config  string            `json:"config,omitempty"`
	Inline  string            `json:"inline,omitempty"`
	Windows *WindowsInstaller `json:"windows,omitempty"`
}

// WindowsInstaller supplies options for a generated autounattend.xml.
type WindowsInstaller struct {
	Locale        string `json:"locale,omitempty"`
	InputLocale   string `json:"input_locale,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	ImageIndex    int    `json:"image_index,omitempty"`
	AdminPassword string `json:"admin_password,omitempty"`
}

// Disk describes the VM's primary disk.
type Disk struct {
	Size   int    `json:"size"`             // in GiB
	Path   string `json:"path,omitempty"`   // optional explicit path
	Format string `json:"format,omitempty"` // auto, qcow2, raw
	Bus    string `json:"bus,omitempty"`    // auto, virtio, sata, scsi
}

// TPM describes an emulated TPM device.
type TPM struct {
	Version string `json:"version,omitempty"` // 1.2, 2.0
	Backend string `json:"backend,omitempty"` // emulator
}

func (c *Config) diskBus() string {
	switch c.Disk.Bus {
	case "", diskBusAuto:
		return ""
	default:
		return c.Disk.Bus
	}
}

func (c *Config) tpmBackend() string {
	if c.TPM != nil && strings.TrimSpace(c.TPM.Backend) != "" {
		return c.TPM.Backend
	}
	return tpmBackendEmulator
}

func (c *Config) tpmVersion() string {
	if c.TPM != nil && strings.TrimSpace(c.TPM.Version) != "" {
		return c.TPM.Version
	}
	return tpmVersion20
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
func Load(path string) (*Config, error) {
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
		cfg.ConfigDir = filepath.Dir(abs)
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
	if c.SecureBoot && c.Firmware != firmwareUEFI {
		return fmt.Errorf("secure_boot requires firmware %q", firmwareUEFI)
	}
	if c.TPM != nil {
		if c.Firmware != firmwareUEFI {
			return fmt.Errorf("tpm requires firmware %q", firmwareUEFI)
		}
		switch c.tpmBackend() {
		case tpmBackendEmulator:
		default:
			return fmt.Errorf("tpm.backend must be %q", tpmBackendEmulator)
		}
		switch c.tpmVersion() {
		case tpmVersion12, tpmVersion20:
		default:
			return fmt.Errorf("tpm.version must be %q or %q", tpmVersion12, tpmVersion20)
		}
	}

	if err := c.validateSource(); err != nil {
		return err
	}

	switch c.Disk.Format {
	case "", diskFormatAuto, diskFormatQCOW2, diskFormatRaw:
	default:
		return fmt.Errorf("disk.format must be %q, %q, or %q", diskFormatAuto, diskFormatQCOW2, diskFormatRaw)
	}
	switch c.Disk.Bus {
	case "", diskBusAuto, diskBusVirtio, diskBusSATA, diskBusSCSI:
	default:
		return fmt.Errorf("disk.bus must be %q, %q, %q, or %q", diskBusAuto, diskBusVirtio, diskBusSATA, diskBusSCSI)
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
	if c.Source.Installer != nil && c.Source.Installer.Type == installer.WindowsAutoUnattend && !c.needsSSH() {
		return fmt.Errorf("installer.type %q requires ssh.key or a provisioner", installer.WindowsAutoUnattend)
	}
	if len(c.Provisioners) > 0 {
		if c.Network.Mode == networkModeUser {
			return fmt.Errorf("provisioners require a guest-visible network; network.mode %q is not supported", networkModeUser)
		}
	}
	for i, p := range c.Provisioners {
		switch p.Type {
		case "", "shell", "powershell":
		default:
			return fmt.Errorf("provisioners[%d].type must be %q or %q", i, "shell", "powershell")
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
		for j, u := range p.Uploads {
			if strings.TrimSpace(u.Source) == "" {
				return fmt.Errorf("provisioners[%d].uploads[%d].source is required", i, j)
			}
			if strings.TrimSpace(u.Target) == "" {
				return fmt.Errorf("provisioners[%d].uploads[%d].target is required", i, j)
			}
		}
		for key := range p.Env {
			if !validEnvName(key) {
				return fmt.Errorf("provisioners[%d].env key %q is invalid", i, key)
			}
		}
	}

	return nil
}

func validEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
		if i == 0 && r >= '0' && r <= '9' {
			ok = false
		}
		if !ok {
			return false
		}
	}
	return true
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
		if _, err := validateMediaSource(src, sourceCloudImage); err != nil {
			return err
		}
		if src.ExtraArgs != "" {
			return fmt.Errorf("source.extra_args is not valid for source.type %q", sourceCloudImage)
		}
		if src.Installer != nil {
			return fmt.Errorf("source.installer requires source.type %q", sourceISO)
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
		if _, err := validateMediaSource(src, sourceISO); err != nil {
			return err
		}
		if src.CloudInit != "" || src.ExtraArgs != "" {
			return fmt.Errorf("source.cloud_init and source.extra_args are not valid for source.type %q", sourceISO)
		}
		if err := validateInstaller(src.Installer); err != nil {
			return err
		}
		if c.Disk.Size <= 0 {
			return fmt.Errorf("disk.size must be greater than zero")
		}

	case sourceDisk:
		if _, err := validateMediaSource(src, sourceDisk); err != nil {
			return err
		}
		if src.CloudInit != "" || src.ExtraArgs != "" {
			return fmt.Errorf("source.cloud_init and source.extra_args are not valid for source.type %q", sourceDisk)
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

func validateMediaSource(src Source, sourceType string) (bool, error) {
	hasPath := strings.TrimSpace(src.Path) != ""
	hasURL := strings.TrimSpace(src.URL) != ""
	if hasPath == hasURL {
		return false, fmt.Errorf("source.type %q requires exactly one of source.path or source.url", sourceType)
	}
	if hasURL {
		if !validSHA256(src.SHA256) {
			return false, fmt.Errorf("source.sha256 must be a 64-character hex digest for URL media; check the SHA256SUMS file next to the URL")
		}
	} else if src.SHA256 != "" && !validSHA256(src.SHA256) {
		return false, fmt.Errorf("source.sha256 must be a 64-character hex digest")
	}
	if src.Cache != "" && !hasURL {
		return false, fmt.Errorf("source.cache requires source.url")
	}
	return hasURL, nil
}

func validateInstaller(inst *Installer) error {
	if inst == nil {
		return nil
	}
	if strings.TrimSpace(inst.Type) == "" {
		return fmt.Errorf("source.installer.type is required")
	}
	impl, err := installer.Lookup(inst.Type)
	if err != nil {
		return fmt.Errorf("source.installer: %w", err)
	}
	if inst.Windows != nil && inst.Type != installer.WindowsAutoUnattend {
		return fmt.Errorf("source.installer.windows is only valid for installer.type %q", installer.WindowsAutoUnattend)
	}
	if inst.Windows != nil && inst.Windows.ImageIndex < 0 {
		return fmt.Errorf("source.installer.windows.image_index must be greater than zero")
	}
	hasConfig := strings.TrimSpace(inst.Config) != ""
	hasInline := inst.Inline != ""
	if hasConfig && hasInline {
		return fmt.Errorf("source.installer requires at most one of config or inline")
	}
	if req, ok := impl.(installer.AnswerFileRequirement); ok && req.RequiresAnswerFile() && !hasConfig && !hasInline {
		return fmt.Errorf("source.installer.type %q requires config or inline", inst.Type)
	}
	return nil
}

// detectImageFormat asks qemu-img for a disk image's format.
func detectImageFormat(path string) (string, error) {
	cmd := exec.Command("qemu-img", "info", "--output=json", path)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("detect format of %q: %w", path, err)
	}

	var info struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", fmt.Errorf("parse qemu-img info for %q: %w", path, err)
	}
	if info.Format == "" {
		return "", fmt.Errorf("qemu-img reported no format for %q", path)
	}
	return info.Format, nil
}

// overlayPath returns the qcow2 overlay path used for a cloud image.
func (c *Config) overlayPath() (string, error) {
	if c.Source.Type != sourceCloudImage {
		return "", fmt.Errorf("overlay is only used with source.type %q", sourceCloudImage)
	}
	if strings.TrimSpace(c.Disk.Path) != "" {
		return filepath.Abs(c.Disk.Path)
	}

	image, err := filepath.Abs(c.Source.Path)
	if err != nil {
		return "", fmt.Errorf("resolve image %q: %w", c.Source.Path, err)
	}
	return filepath.Join(filepath.Dir(image), c.Name+"-overlay.qcow2"), nil
}

// seedPath returns the NoCloud seed ISO path for a cloud image.
func (c *Config) seedPath() (string, error) {
	overlay, err := c.overlayPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(overlay), c.Name+"-seed.iso"), nil
}

// baseFormat is the backing format used when creating the overlay.
func (c *Config) baseFormat() (string, error) {
	switch c.Disk.Format {
	case "", diskFormatAuto:
		return detectImageFormat(c.Source.Path)
	case diskFormatQCOW2, diskFormatRaw:
		return c.Disk.Format, nil
	default:
		return "", fmt.Errorf("unsupported disk.format %q", c.Disk.Format)
	}
}

// newDiskFormat is the format for a newly created primary disk.
func (c *Config) newDiskFormat() string {
	if c.Disk.Format == diskFormatRaw {
		return diskFormatRaw
	}
	return diskFormatQCOW2
}

// sourceDiskFormat is the format of an imported source disk.
func (c *Config) sourceDiskFormat() (string, error) {
	switch c.Disk.Format {
	case "", diskFormatAuto:
		return detectImageFormat(c.Source.Path)
	case diskFormatQCOW2, diskFormatRaw:
		return c.Disk.Format, nil
	default:
		return "", fmt.Errorf("unsupported disk.format %q", c.Disk.Format)
	}
}

// extraDiskFormat is the format of an attached extra disk.

func (c *Config) installerSpec() (installer.Spec, error) {
	if !c.hasInstaller() {
		return installer.Spec{}, nil
	}
	impl, err := installer.Lookup(c.Source.Installer.Type)
	if err != nil {
		return installer.Spec{}, err
	}
	answer, err := c.installerAnswerFile()
	if err != nil {
		return installer.Spec{}, err
	}
	input := installer.Input{
		Hostname:   c.Name,
		SSHUser:    c.sshUser(),
		SSHKey:     c.SSHPublicKey,
		AnswerFile: answer,
	}
	if c.Source.Installer.Type == installer.WindowsAutoUnattend {
		windows := installer.WindowsOptions{Firmware: c.Firmware}
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
				return installer.Spec{}, err
			}
			windows.AdminPassword = password
		}
		input.Windows = &windows
	}
	return impl.Spec(input)
}

func (c *Config) windowsAdminPassword() (string, error) {
	if c.WindowsPassword != "" {
		return c.WindowsPassword, nil
	}
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate Windows administrator password: %w", err)
	}
	password := "A!9" + hex.EncodeToString(raw)
	c.WindowsPassword = password
	if strings.TrimSpace(c.AssetDir) != "" {
		if err := os.MkdirAll(c.AssetDir, 0o755); err != nil {
			return "", fmt.Errorf("create asset dir: %w", err)
		}
		path := filepath.Join(c.AssetDir, "windows-admin-password")
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
		if c.ConfigDir != "" {
			path = filepath.Join(c.ConfigDir, path)
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
	if strings.TrimSpace(c.AssetDir) == "" {
		return "", fmt.Errorf("installer seed path requires the project asset directory")
	}
	return filepath.Join(c.AssetDir, c.Name+"-installer-seed.iso"), nil
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
	if strings.TrimSpace(c.AssetDir) == "" {
		return "", fmt.Errorf("remastered installer ISO requires the project asset directory")
	}
	return filepath.Join(c.AssetDir, c.Name+"-installer.iso"), nil
}

func Hash(cfg *Config) (string, error) {
	// Provisioning, SSH, and state-location settings are not VM shape;
	// changing a script or moving state must not require recreating the domain.
	clone := *cfg
	clone.StateDir = ""
	clone.Autostart = false
	clone.SSH = SSHConfig{}
	clone.Provisioners = nil
	data, err := json.Marshal(&clone)
	if err != nil {
		return "", fmt.Errorf("hash config: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// applyProjectDefaults makes the managed disk location deterministic.
func ApplyProjectDefaults(cfg *Config, layout state.ProjectLayout) {
	cfg.AssetDir = layout.AssetDir
	if cfg.Source.Type == sourceCloudImage && strings.TrimSpace(cfg.Disk.Path) == "" {
		cfg.Disk.Path = filepath.Join(layout.AssetDir, "disk.qcow2")
	}
}

// Exported accessors and aliases used by the CLI while it is split out of
// package main.
const (
	SourceNone        = sourceNone
	SourceCloudImage  = sourceCloudImage
	SourceInstallTree = sourceInstallTree
	SourceISO         = sourceISO
	SourceDisk        = sourceDisk
	SourcePXE         = sourcePXE

	FirmwareBIOS = firmwareBIOS
	FirmwareUEFI = firmwareUEFI

	MountType9P       = mountType9P
	MountTypeNone     = mountTypeNone
	MountTypeVirtioFS = mountTypeVirtioFS

	NetworkModeAuto    = networkModeAuto
	NetworkModeDefault = networkModeDefault
	NetworkModeUser    = networkModeUser
	NetworkModeBridge  = networkModeBridge

	GraphicsNone  = graphicsNone
	GraphicsVNC   = graphicsVNC
	GraphicsSPICE = graphicsSPICE

	DiskFormatAuto  = diskFormatAuto
	DiskFormatQCOW2 = diskFormatQCOW2
	DiskFormatRaw   = diskFormatRaw

	DiskBusAuto   = diskBusAuto
	DiskBusVirtio = diskBusVirtio
	DiskBusSATA   = diskBusSATA
	DiskBusSCSI   = diskBusSCSI

	TPMBackendEmulator = tpmBackendEmulator
	TPMVersion12       = tpmVersion12
	TPMVersion20       = tpmVersion20
)

func (c *Config) SSHUser() string                        { return c.sshUser() }
func (c *Config) NeedsSSH() bool                         { return c.needsSSH() }
func (c *Config) NeedsSeed() bool                        { return c.needsSeed() }
func (c *Config) HasInstaller() bool                     { return c.hasInstaller() }
func (c *Config) CanGenerateSSHKey() bool                { return c.canGenerateSSHKey() }
func (c *Config) DiskBus() string                        { return c.diskBus() }
func (c *Config) TPMBackend() string                     { return c.tpmBackend() }
func (c *Config) TPMVersion() string                     { return c.tpmVersion() }
func (c *Config) InstallerSpec() (installer.Spec, error) { return c.installerSpec() }
func (c *Config) WindowsAdminPassword() (string, error)  { return c.windowsAdminPassword() }
func (c *Config) InstallerAnswerFile() ([]byte, error)   { return c.installerAnswerFile() }
func (c *Config) InstallerSeedPath() (string, error)     { return c.installerSeedPath() }
func (c *Config) InstallerISOPath() (string, error)      { return c.installerISOPath() }
func (c *Config) OverlayPath() (string, error)           { return c.overlayPath() }
func (c *Config) SeedPath() (string, error)              { return c.seedPath() }
func (c *Config) BaseFormat() (string, error)            { return c.baseFormat() }
func (c *Config) NewDiskFormat() string                  { return c.newDiskFormat() }
func (c *Config) SourceDiskFormat() (string, error)      { return c.sourceDiskFormat() }

func DetectImageFormat(path string) (string, error) { return detectImageFormat(path) }

// ValidSHA256 reports whether s is a SHA-256 hex digest.
func ValidSHA256(s string) bool { return validSHA256(s) }

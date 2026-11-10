package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

// CheckImageTools verifies the host tools required by the configured source.
func CheckImageTools(cfg *Config) error {
	needsQEMUImg := cfg.Source.Type == sourceCloudImage
	if cfg.Source.Type == sourceDisk && (cfg.Disk.Format == "" || cfg.Disk.Format == diskFormatAuto) {
		needsQEMUImg = true
	}
	for _, d := range cfg.ExtraDisks {
		if d.Format == "" || d.Format == diskFormatAuto {
			needsQEMUImg = true
		}
	}

	if needsQEMUImg {
		if _, err := exec.LookPath("qemu-img"); err != nil {
			return fmt.Errorf("required binary \"qemu-img\" not found in PATH: %w", err)
		}
	}

	if strings.TrimSpace(cfg.Source.CloudInit) != "" {
		found := false
		for _, bin := range []string{"xorrisofs", "genisoimage", "mkisofs"} {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("cloud_init requires xorrisofs, genisoimage, or mkisofs in PATH")
		}
	}

	return nil
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
func extraDiskFormat(d ExtraDisk) (string, error) {
	switch d.Format {
	case "", diskFormatAuto:
		return detectImageFormat(d.Path)
	case diskFormatQCOW2, diskFormatRaw:
		return d.Format, nil
	default:
		return "", fmt.Errorf("unsupported extra disk format %q", d.Format)
	}
}

// isoBuilder picks an ISO creation tool compatible with virt-install's
// cloud-init seed flags.
func isoBuilder() (string, error) {
	for _, name := range []string{"xorrisofs", "genisoimage", "mkisofs"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("cloud_init requires xorrisofs, genisoimage, or mkisofs in PATH")
}

// prepareOverlay creates the copy-on-write overlay for a cloud image. It never
// overwrites an implicit disk path; an explicit disk.path is user-managed and
// reused.
func prepareOverlay(cfg *Config) (string, error) {
	if cfg.Source.Type != sourceCloudImage {
		return "", nil
	}

	image, err := filepath.Abs(cfg.Source.Path)
	if err != nil {
		return "", fmt.Errorf("resolve image %q: %w", cfg.Source.Path, err)
	}
	if _, err := os.Stat(image); err != nil {
		return "", fmt.Errorf("image %q is not accessible: %w", cfg.Source.Path, err)
	}

	overlay, err := cfg.overlayPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(overlay); err == nil {
		if strings.TrimSpace(cfg.Disk.Path) != "" {
			return overlay, nil
		}
		return "", fmt.Errorf("disk image %q already exists; remove it or choose another disk.path", overlay)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check disk image %q: %w", overlay, err)
	}

	baseFormat, err := cfg.baseFormat()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(overlay), 0o755); err != nil {
		return "", fmt.Errorf("create disk directory: %w", err)
	}

	cmd := exec.Command("qemu-img", "create",
		"-f", diskFormatQCOW2,
		"-F", baseFormat,
		"-b", image,
		overlay,
		fmt.Sprintf("%dG", cfg.Disk.Size),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create disk image %q: %w: %s", overlay, err, strings.TrimSpace(string(out)))
	}

	return overlay, nil
}

// prepareSeed creates a persistent NoCloud seed ISO. Using our own seed avoids
// virt-install's transient cloud-init ISO and its forced on_reboot=destroy.
func prepareSeed(cfg *Config) (string, error) {
	if strings.TrimSpace(cfg.Source.CloudInit) == "" {
		return "", nil
	}

	seed, err := cfg.seedPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(seed); err == nil {
		return seed, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check cloud-init seed %q: %w", seed, err)
	}

	userData, err := filepath.Abs(cfg.Source.CloudInit)
	if err != nil {
		return "", fmt.Errorf("resolve cloud_init %q: %w", cfg.Source.CloudInit, err)
	}
	content, err := os.ReadFile(userData)
	if err != nil {
		return "", fmt.Errorf("cloud_init %q is not accessible: %w", cfg.Source.CloudInit, err)
	}

	tool, err := isoBuilder()
	if err != nil {
		return "", err
	}

	dir, err := os.MkdirTemp("", "vmlite-seed-")
	if err != nil {
		return "", fmt.Errorf("create seed workspace: %w", err)
	}
	defer os.RemoveAll(dir)

	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", cfg.Name, cfg.Name)
	if err := os.WriteFile(filepath.Join(dir, "meta-data"), []byte(metaData), 0o644); err != nil {
		return "", fmt.Errorf("write seed meta-data: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user-data"), content, 0o644); err != nil {
		return "", fmt.Errorf("write seed user-data: %w", err)
	}

	cmd := exec.Command(tool,
		"-o", seed,
		"-J",
		"-input-charset", "utf8",
		"-rational-rock",
		"-V", "cidata",
		dir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create cloud-init seed %q: %w: %s", seed, err, strings.TrimSpace(string(out)))
	}
	if err := os.Chmod(seed, 0o644); err != nil {
		return "", fmt.Errorf("chmod cloud-init seed %q: %w", seed, err)
	}

	return seed, nil
}

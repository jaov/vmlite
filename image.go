package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CheckImageTools verifies the host tools required by image-based configs.
func CheckImageTools(cfg *Config) error {
	if strings.TrimSpace(cfg.Image) != "" {
		if _, err := exec.LookPath("qemu-img"); err != nil {
			return fmt.Errorf("required binary \"qemu-img\" not found in PATH: %w", err)
		}
	}

	if strings.TrimSpace(cfg.CloudInit) != "" {
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

// overlayPath returns the qcow2 overlay path used for an image-based VM.
func (c *Config) overlayPath() (string, error) {
	if strings.TrimSpace(c.DiskPath) != "" {
		return filepath.Abs(c.DiskPath)
	}

	image, err := filepath.Abs(c.Image)
	if err != nil {
		return "", fmt.Errorf("resolve image %q: %w", c.Image, err)
	}
	return filepath.Join(filepath.Dir(image), c.Name+"-overlay.qcow2"), nil
}

// seedPath returns the NoCloud seed ISO path for cfg.
func (c *Config) seedPath() (string, error) {
	overlay, err := c.overlayPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(overlay), c.Name+"-seed.iso"), nil
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

// prepareSeed creates a persistent NoCloud seed ISO. Using our own seed avoids
// virt-install's transient cloud-init ISO and its forced on_reboot=destroy.
func prepareSeed(cfg *Config) (string, error) {
	if strings.TrimSpace(cfg.CloudInit) == "" {
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

	userData, err := filepath.Abs(cfg.CloudInit)
	if err != nil {
		return "", fmt.Errorf("resolve cloud_init %q: %w", cfg.CloudInit, err)
	}
	content, err := os.ReadFile(userData)
	if err != nil {
		return "", fmt.Errorf("cloud_init %q is not accessible: %w", cfg.CloudInit, err)
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

// prepareOverlay creates the copy-on-write overlay for cfg.Image. It never
// overwrites an existing disk image.
func prepareOverlay(cfg *Config) (string, error) {
	if strings.TrimSpace(cfg.Image) == "" {
		return "", nil
	}

	image, err := filepath.Abs(cfg.Image)
	if err != nil {
		return "", fmt.Errorf("resolve image %q: %w", cfg.Image, err)
	}
	if _, err := os.Stat(image); err != nil {
		return "", fmt.Errorf("image %q is not accessible: %w", cfg.Image, err)
	}

	overlay, err := cfg.overlayPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(overlay); err == nil {
		// An explicitly configured disk_path is treated as a user-managed disk
		// and reused. The implicit default path must not silently shadow an
		// existing image.
		if strings.TrimSpace(cfg.DiskPath) != "" {
			return overlay, nil
		}
		return "", fmt.Errorf("disk image %q already exists; remove it or choose another disk_path", overlay)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check disk image %q: %w", overlay, err)
	}

	cmd := exec.Command("qemu-img", "create",
		"-f", "qcow2",
		"-F", "qcow2",
		"-b", image,
		overlay,
		fmt.Sprintf("%dG", cfg.DiskSize),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create disk image %q: %w: %s", overlay, err, strings.TrimSpace(string(out)))
	}

	return overlay, nil
}

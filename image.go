package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var errImageWouldDownload = errors.New("image would be downloaded")

// resolveImage returns a local, verified cloud-image path. URL images are
// downloaded into the cache and checked against source.sha256.
func resolveImage(cfg *Config, offline, dryRun bool) (string, error) {
	if cfg.Source.Type != sourceCloudImage {
		return "", nil
	}

	if strings.TrimSpace(cfg.Source.Path) != "" {
		path, err := filepath.Abs(cfg.Source.Path)
		if err != nil {
			return "", fmt.Errorf("resolve image %q: %w", cfg.Source.Path, err)
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("image %q is not accessible: %w", cfg.Source.Path, err)
		}
		if cfg.Source.SHA256 != "" {
			if err := verifySHA256(path, cfg.Source.SHA256); err != nil {
				return "", fmt.Errorf("image %q: %w", cfg.Source.Path, err)
			}
		}
		return path, nil
	}

	if !validSHA256(cfg.Source.SHA256) {
		return "", fmt.Errorf("source.sha256 is required for URL images")
	}
	sha := strings.ToLower(cfg.Source.SHA256)

	cacheDir := cfg.Source.Cache
	if strings.TrimSpace(cacheDir) == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("locate image cache: %w", err)
		}
		cacheDir = filepath.Join(userCache, "vmlite")
	}
	cacheDir, err := filepath.Abs(cacheDir)
	if err != nil {
		return "", fmt.Errorf("resolve image cache %q: %w", cacheDir, err)
	}

	parsed, err := url.Parse(cfg.Source.URL)
	if err != nil {
		return "", fmt.Errorf("parse image URL %q: %w", cfg.Source.URL, err)
	}
	base := filepath.Base(parsed.Path)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "image.qcow2"
	}
	cachePath := filepath.Join(cacheDir, sha[:16]+"-"+base)

	if _, err := os.Stat(cachePath); err == nil {
		if err := verifySHA256(cachePath, sha); err == nil {
			return cachePath, nil
		}
		if offline {
			return "", fmt.Errorf("cached image %q does not match source.sha256 and --offline is set", cachePath)
		}
		if err := os.Remove(cachePath); err != nil {
			return "", fmt.Errorf("remove mismatched cached image %q: %w", cachePath, err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check cached image %q: %w", cachePath, err)
	}

	if offline {
		return "", fmt.Errorf("image is not cached at %q and --offline is set", cachePath)
	}
	if dryRun {
		return "", errImageWouldDownload
	}
	if err := downloadImage(cfg.Source.URL, cachePath, sha); err != nil {
		return "", err
	}
	return cachePath, nil
}

func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, want)
	}
	return nil
}

func downloadImage(rawURL, path, wantSHA string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create image cache: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Get(rawURL)
	if err != nil {
		return fmt.Errorf("download image %q: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download image %q: unexpected HTTP status %s", rawURL, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return fmt.Errorf("create download temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("download image %q: %w", rawURL, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close downloaded image: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, wantSHA) {
		return fmt.Errorf("image %q sha256 mismatch: got %s, want %s", rawURL, got, wantSHA)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod downloaded image: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install downloaded image %q: %w", path, err)
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

	if cfg.needsSeed() {
		found := false
		for _, bin := range []string{"xorrisofs", "genisoimage", "mkisofs"} {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("cloud_init/provisioners require xorrisofs, genisoimage, or mkisofs in PATH")
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
	if !cfg.needsSeed() {
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

	var content []byte
	if strings.TrimSpace(cfg.Source.CloudInit) != "" {
		userData, err := filepath.Abs(cfg.Source.CloudInit)
		if err != nil {
			return "", fmt.Errorf("resolve cloud_init %q: %w", cfg.Source.CloudInit, err)
		}
		content, err = os.ReadFile(userData)
		if err != nil {
			return "", fmt.Errorf("cloud_init %q is not accessible: %w", cfg.Source.CloudInit, err)
		}
	} else {
		content = []byte("#cloud-config\n")
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
	if cfg.needsSSH() {
		if strings.TrimSpace(cfg.sshPublicKey) == "" {
			return "", fmt.Errorf("SSH public key is not available; generate keys before creating the seed")
		}
		vendor := fmt.Sprintf(`#cloud-config
users:
  - name: %s
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: true
    ssh_authorized_keys:
      - %s
ssh_pwauth: false
`, cfg.sshUser(), strings.TrimSpace(cfg.sshPublicKey))
		if err := os.WriteFile(filepath.Join(dir, "vendor-data"), []byte(vendor), 0o644); err != nil {
			return "", fmt.Errorf("write seed vendor-data: %w", err)
		}
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

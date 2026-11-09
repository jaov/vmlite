//go:build integration

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type bootProfile struct {
	OSVariant  string
	SSHUser    string
	OS         string
	SudoGroup  string
	Shell      string
	Mount      string // linux9p, freebsd9p, none
	CloudInit  bool
	WorkRoot   string
	NoStage    bool
	SSHTimeout time.Duration
}

var bootProfiles = map[string]bootProfile{
	"ubuntu": {
		OSVariant: "ubuntu24.04", SSHUser: "vmlite", OS: "Linux",
		SudoGroup: "sudo", Shell: "/bin/bash", Mount: "linux9p", CloudInit: true,
		WorkRoot: "/var/tmp", SSHTimeout: 5 * time.Minute,
	},
	"debian": {
		OSVariant: "debian12", SSHUser: "vmlite", OS: "Linux",
		SudoGroup: "sudo", Shell: "/bin/bash", Mount: "linux9p", CloudInit: true,
		WorkRoot: "/var/tmp", SSHTimeout: 5 * time.Minute,
	},
	"fedora": {
		OSVariant: "fedora42", SSHUser: "vmlite", OS: "Linux",
		SudoGroup: "wheel", Shell: "/bin/bash", Mount: "linux9p", CloudInit: true,
		WorkRoot: "/var/tmp", SSHTimeout: 5 * time.Minute,
	},
	"arch": {
		OSVariant: "archlinux", SSHUser: "vmlite", OS: "Linux",
		SudoGroup: "wheel", Shell: "/bin/bash", Mount: "linux9p", CloudInit: true,
		WorkRoot: "/var/tmp", SSHTimeout: 5 * time.Minute,
	},
	"freebsd": {
		OSVariant: "freebsd14.2", SSHUser: "vmlite", OS: "FreeBSD",
		SudoGroup: "wheel", Shell: "/bin/sh", Mount: "none", CloudInit: false,
		WorkRoot: "/tmp", NoStage: true, SSHTimeout: 12 * time.Minute,
	},
}

// requireBootIntegration returns the cloud image path when the host can run a
// bootable guest test, and skips otherwise.
func requireBootIntegration(t *testing.T) string {
	t.Helper()

	image := os.Getenv("VMLITE_TEST_IMAGE")
	if image == "" {
		t.Skip("set VMLITE_TEST_IMAGE to a qcow2 cloud image (see TESTING.md)")
	}
	if _, err := os.Stat(image); err != nil {
		t.Skipf("VMLITE_TEST_IMAGE is not accessible: %v", err)
	}

	for _, bin := range []string{"virsh", "virt-install", "qemu-img", "ssh", "scp", "ssh-keygen"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("required binary %q not found in PATH", bin)
		}
	}

	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt is unavailable: %v", err)
	}

	return image
}

func freeBytes(t *testing.T, path string) uint64 {
	t.Helper()
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		t.Fatalf("statfs %s: %v", path, err)
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize)
}

func stageImage(t *testing.T, src, dst string) {
	t.Helper()
	// Copy rather than hardlink: libvirt's dynamic ownership can chown the
	// backing chain, which would otherwise change the user's cached image.
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open image: %v", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create staged image: %v", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy image: %v", err)
	}
	if err := os.Chmod(dst, 0o644); err != nil {
		t.Fatalf("chmod staged image: %v", err)
	}
}

func sshGuest(t *testing.T, userName, host, key, command string, timeout time.Duration) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{
		"-i", key,
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "LogLevel=ERROR",
		userName + "@" + host,
		command,
	}
	out, err := exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
	return string(out), err
}

func domainIPv4(t *testing.T, uri, name string) string {
	t.Helper()
	out, err := virsh(t, uri, "domifaddr", name, "--source", "lease")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			if field == "ipv4" && i+1 < len(fields) {
				return strings.Split(fields[i+1], "/")[0]
			}
		}
	}
	return ""
}

// waitForGuestSSH re-resolves the DHCP lease on every attempt, so it survives
// the first-boot package-update reboot some images perform.
func waitForGuestSSH(t *testing.T, uri, name, userName, key string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastIP, lastOut string
	for time.Now().Before(deadline) {
		ip := domainIPv4(t, uri, name)
		if ip != "" {
			lastIP = ip
			out, err := sshGuest(t, userName, ip, key, "true", 10*time.Second)
			if err == nil {
				return ip
			}
			lastOut = out
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("SSH to %s not ready after %s (last IP %q):\n%s", userName, timeout, lastIP, lastOut)
	return ""
}

func scpGuest(t *testing.T, userName, host, key, src, dst string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{
		"-i", key,
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "LogLevel=ERROR",
		src,
		userName + "@" + host + ":" + dst,
	}
	out, err := exec.CommandContext(ctx, "scp", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp: %w: %s", err, out)
	}
	return nil
}

func TestBootableGuest(t *testing.T) {
	imageSrc := requireBootIntegration(t)

	profileName := envOr("VMLITE_TEST_PROFILE", "ubuntu")
	profile, ok := bootProfiles[profileName]
	if !ok {
		t.Fatalf("unknown VMLITE_TEST_PROFILE %q", profileName)
	}
	variant := envOr("VMLITE_TEST_OS_VARIANT", profile.OSVariant)
	sshUser := envOr("VMLITE_TEST_SSH_USER", profile.SSHUser)
	workRoot := envOr("VMLITE_TEST_WORKDIR", profile.WorkRoot)
	if workRoot == "" {
		workRoot = "/var/tmp"
	}
	noStage := profile.NoStage
	if v := os.Getenv("VMLITE_TEST_NO_STAGE"); v != "" {
		noStage = v == "1"
	}
	sshTimeout := profile.SSHTimeout
	if sshTimeout <= 0 {
		sshTimeout = 5 * time.Minute
	}

	workDir, err := os.MkdirTemp(workRoot, "vmlite-it-boot-")
	if err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		t.Fatalf("resolve work dir: %v", err)
	}
	if err := os.Chmod(workDir, 0o755); err != nil {
		t.Fatalf("chmod work dir: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			t.Logf("VMLITE_TEST_KEEP=1: leaving work dir %s", workDir)
			return
		}
		// virt-install creates a transient directory pool for an explicit
		// disk path; remove it along with the work dir.
		pool := filepath.Base(workDir)
		_, _ = virsh(t, systemURI, "pool-destroy", pool)
		_, _ = virsh(t, systemURI, "pool-undefine", pool)
		os.RemoveAll(workDir)
	})

	minFree := uint64(3 << 30)
	if noStage {
		minFree = 2 << 30
	}
	if free := freeBytes(t, workDir); free < minFree {
		t.Skipf("not enough free space in %s: %d bytes available, want at least %d GiB",
			workDir, free, minFree>>30)
	}

	sharedDir := filepath.Join(workDir, "shared")
	if err := os.Mkdir(sharedDir, 0o755); err != nil {
		t.Fatalf("create shared dir: %v", err)
	}
	marker := fmt.Sprintf("vmlite-9p-marker-%d-%d", os.Getpid(), nameCounter.Add(1))
	if err := os.WriteFile(filepath.Join(sharedDir, "marker.txt"), []byte(marker+"\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	keyPath := filepath.Join(workDir, "id_ed25519")
	keygen := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyPath)
	if out, err := keygen.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v: %s", err, out)
	}
	pub, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		t.Fatalf("read public key: %v", err)
	}

	var mountScript string
	switch profile.Mount {
	case "linux9p":
		mountScript = "mkdir -p /mnt/shared && mount -t 9p -o trans=virtio,version=9p2000.L shared_data /mnt/shared && cp /mnt/shared/marker.txt /var/tmp/vmlite-marker.txt && chmod 0644 /var/tmp/vmlite-marker.txt"
	case "freebsd9p":
		mountScript = "kldload virtio_p9fs 2>/dev/null || true; mkdir -p /mnt/shared; mount -t p9fs shared_data /mnt/shared && cp /mnt/shared/marker.txt /var/tmp/vmlite-marker.txt && chmod 0644 /var/tmp/vmlite-marker.txt"
	case "none":
	default:
		t.Fatalf("unknown mount mode %q for profile %q", profile.Mount, profileName)
	}
	runcmd := ""
	if mountScript != "" {
		runcmd = fmt.Sprintf("runcmd:\n  - [sh, -c, '%s']\n", mountScript)
	}

	userData := filepath.Join(workDir, "user-data")
	userDataContent := fmt.Sprintf(`#cloud-config
package_update: false
package_upgrade: false
users:
  - name: %s
    groups: [%s]
    shell: %s
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: true
    ssh_authorized_keys:
      - %s
ssh_pwauth: false
%s`, sshUser, profile.SudoGroup, profile.Shell, strings.TrimSpace(string(pub)), runcmd)
	if err := os.WriteFile(userData, []byte(userDataContent), 0o644); err != nil {
		t.Fatalf("write user-data: %v", err)
	}

	stagedImage := imageSrc
	if !noStage {
		stagedImage = filepath.Join(workDir, "base.qcow2")
		stageImage(t, imageSrc, stagedImage)
	}

	// The overlay is created here with world-writable permissions so the
	// system qemu user can write to it. vmlite reuses an explicit disk_path.
	overlay := filepath.Join(workDir, "overlay.qcow2")
	createOverlay := exec.Command("qemu-img", "create",
		"-f", "qcow2", "-F", "qcow2", "-b", stagedImage, overlay, "10G")
	if out, err := createOverlay.CombinedOutput(); err != nil {
		t.Fatalf("create overlay: %v: %s", err, out)
	}
	if err := os.Chmod(overlay, 0o666); err != nil {
		t.Fatalf("chmod overlay: %v", err)
	}

	name := uniqueName()
	registerCleanup(t, systemURI, name)

	cfg := Config{
		Name:      name,
		Memory:    2048,
		CPUs:      2,
		OSVariant: variant,
		Firmware:  firmwareBIOS,
		Source: Source{
			Type:      sourceCloudImage,
			Path:      stagedImage,
			CloudInit: userData,
		},
		Disk:     Disk{Size: 10, Path: overlay, Format: diskFormatQCOW2},
		Network:  Network{Mode: networkModeAuto},
		Graphics: Graphics{Type: graphicsNone},
		Mounts:   []Mount{{Source: sharedDir, Target: "shared_data", Type: mountType9P}},
	}
	configPath := writeConfig(t, cfg)

	out, err := runVmlite(t, systemURI, configPath)
	if err != nil {
		t.Fatalf("vmlite create failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created successfully") {
		t.Fatalf("unexpected vmlite output:\n%s", out)
	}

	xmlOut, err := virsh(t, systemURI, "dumpxml", name)
	if err != nil {
		t.Fatalf("dumpxml: %v\n%s", err, xmlOut)
	}
	for _, want := range []string{
		"-seed.iso",
		"ds=nocloud",
		"network='default'",
		"accessmode='mapped'",
		"<on_reboot>restart</on_reboot>",
	} {
		if !strings.Contains(xmlOut, want) {
			t.Errorf("domain XML missing %q:\n%s", want, xmlOut)
		}
	}

	ip := waitForGuestSSH(t, systemURI, name, sshUser, keyPath, sshTimeout)

	osOut, err := sshGuest(t, sshUser, ip, keyPath, "uname -s", 30*time.Second)
	if err != nil {
		t.Fatalf("detect guest OS: %v\n%s", err, osOut)
	}
	if strings.TrimSpace(osOut) != profile.OS {
		t.Fatalf("guest OS = %q, want %q", strings.TrimSpace(osOut), profile.OS)
	}

	artifact := filepath.Join(workDir, "artifact.txt")
	artifactContent := "artifact-" + marker
	if err := os.WriteFile(artifact, []byte(artifactContent+"\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	remoteArtifact := "/tmp/vmlite-artifact-" + name
	if err := scpGuest(t, sshUser, ip, keyPath, artifact, remoteArtifact); err != nil {
		t.Fatalf("scp artifact: %v", err)
	}
	artifactOut, err := sshGuest(t, sshUser, ip, keyPath, "cat "+remoteArtifact, 30*time.Second)
	if err != nil {
		t.Fatalf("read artifact: %v\n%s", err, artifactOut)
	}
	if !strings.Contains(artifactOut, artifactContent) {
		t.Fatalf("artifact %q not found in guest output:\n%s", artifactContent, artifactOut)
	}

	if profile.CloudInit {
		cloudOut, err := sshGuest(t, sshUser, ip, keyPath,
			"cloud-init status --wait && cloud-init status --long", 3*time.Minute)
		if err != nil {
			t.Fatalf("cloud-init checks failed: %v\n%s", err, cloudOut)
		}
		if !strings.Contains(cloudOut, "status: done") {
			t.Fatalf("cloud-init did not report success:\n%s", cloudOut)
		}
	}

	if profile.Mount != "none" {
		deadline := time.Now().Add(2 * time.Minute)
		var last string
		for time.Now().Before(deadline) {
			out, err := sshGuest(t, sshUser, ip, keyPath, "cat /var/tmp/vmlite-marker.txt", 15*time.Second)
			last = out
			if err == nil && strings.Contains(out, marker) {
				return
			}
			time.Sleep(2 * time.Second)
		}
		t.Fatalf("9p marker %q not found in guest (runcmd mount failed?):\n%s", marker, last)
	}
}

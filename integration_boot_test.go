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

	for _, bin := range []string{"virsh", "virt-install", "qemu-img", "ssh", "ssh-keygen"} {
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

func waitForSSH(t *testing.T, userName, host, key string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastOut string
	var lastErr error
	for time.Now().Before(deadline) {
		lastOut, lastErr = sshGuest(t, userName, host, key, "true", 10*time.Second)
		if lastErr == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("SSH to %s@%s not ready after %s: %v\n%s", userName, host, timeout, lastErr, lastOut)
}

func waitForLease(t *testing.T, uri, name string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		out, err := virsh(t, uri, "domifaddr", name, "--source", "lease")
		last = out
		if err == nil {
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "ipv4" && i+1 < len(fields) {
						return strings.Split(fields[i+1], "/")[0]
					}
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("no DHCP lease for %s within %s:\n%s", name, timeout, last)
	return ""
}

func TestBootableGuest(t *testing.T) {
	imageSrc := requireBootIntegration(t)

	variant := envOr("VMLITE_TEST_OS_VARIANT", "ubuntu24.04")
	sshUser := envOr("VMLITE_TEST_SSH_USER", "vmlite")
	workRoot := envOr("VMLITE_TEST_WORKDIR", "/var/tmp")

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
		os.RemoveAll(workDir)
	})

	if free := freeBytes(t, workDir); free < 3<<30 {
		t.Skipf("not enough free space in %s: %d bytes available, want at least 3 GiB", workDir, free)
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

	userData := filepath.Join(workDir, "user-data")
	userDataContent := fmt.Sprintf(`#cloud-config
users:
  - name: %s
    groups: [sudo]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: true
    ssh_authorized_keys:
      - %s
ssh_pwauth: false
package_update: false
`, sshUser, strings.TrimSpace(string(pub)))
	if err := os.WriteFile(userData, []byte(userDataContent), 0o644); err != nil {
		t.Fatalf("write user-data: %v", err)
	}

	stagedImage := filepath.Join(workDir, "base.qcow2")
	stageImage(t, imageSrc, stagedImage)

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
		DiskSize:  10,
		OSVariant: variant,
		Image:     stagedImage,
		DiskPath:  overlay,
		CloudInit: userData,
		Mounts:    []Mount{{Source: sharedDir, Target: "shared_data"}},
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
		"device='cdrom'",
		"ds=nocloud",
		"network='default'",
		"accessmode='mapped'",
		"<on_reboot>restart</on_reboot>",
	} {
		if !strings.Contains(xmlOut, want) {
			t.Errorf("domain XML missing %q:\n%s", want, xmlOut)
		}
	}

	ip := waitForLease(t, systemURI, name, 3*time.Minute)
	waitForSSH(t, sshUser, ip, keyPath, 3*time.Minute)

	remote := `set -e
echo "HOST=$(hostname)"
cloud-init status --wait
cloud-init status --long
sudo modprobe 9pnet_virtio
sudo mkdir -p /mnt/shared
sudo mount -t 9p -o trans=virtio,version=9p2000.L shared_data /mnt/shared
cat /mnt/shared/marker.txt
`
	result, err := sshGuest(t, sshUser, ip, keyPath, remote, 3*time.Minute)
	if err != nil {
		t.Fatalf("guest checks failed: %v\n%s", err, result)
	}
	if !strings.Contains(result, marker) {
		t.Fatalf("marker %q not found in guest output:\n%s", marker, result)
	}
	if !strings.Contains(result, "status: done") {
		t.Fatalf("cloud-init did not report success:\n%s", result)
	}
}

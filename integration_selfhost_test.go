//go:build integration

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func selfhostWorkDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "vmlite-selfhost-")
	if err != nil {
		t.Fatalf("create self-host work dir: %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod self-host work dir: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			t.Logf("VMLITE_TEST_KEEP=1: leaving self-host work dir %s", dir)
			return
		}
		pool := filepath.Base(dir)
		_, _ = virsh(t, systemURI, "pool-destroy", pool)
		_, _ = virsh(t, systemURI, "pool-undefine", pool)
		os.RemoveAll(dir)
	})
	return dir
}

func buildStaticVmlite(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", path, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build static vmlite: %v\n%s", err, out)
	}
}

func tailText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "..." + s[len(s)-max:]
}

func TestVmliteInGuest(t *testing.T) {
	if os.Getenv("VMLITE_TEST_SELFHOST") != "1" {
		t.Skip("set VMLITE_TEST_SELFHOST=1 to run the nested self-host test")
	}
	image := os.Getenv("VMLITE_TEST_SELFHOST_IMAGE")
	if image == "" {
		t.Skip("set VMLITE_TEST_SELFHOST_IMAGE to an Ubuntu cloud image")
	}
	if _, err := os.Stat(image); err != nil {
		t.Skipf("VMLITE_TEST_SELFHOST_IMAGE is not accessible: %v", err)
	}
	for _, bin := range []string{"go", "ssh", "scp", "ssh-keygen", "qemu-img", "virt-install", "virsh"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("required binary %q not found in PATH", bin)
		}
	}
	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt is unavailable: %v", err)
	}

	workDir := selfhostWorkDir(t)
	if free := freeBytes(t, workDir); free < 5<<30 {
		t.Skipf("not enough tmpfs space in %s: %d bytes available, want at least 5 GiB", workDir, free)
	}

	stagedImage := filepath.Join(workDir, "base.qcow2")
	stageImage(t, image, stagedImage)
	overlay := filepath.Join(workDir, "builder-overlay.qcow2")
	create := exec.Command("qemu-img", "create", "-f", "qcow2", "-F", "qcow2", "-b", stagedImage, overlay, "20G")
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create builder overlay: %v: %s", err, out)
	}
	if err := os.Chmod(overlay, 0o666); err != nil {
		t.Fatalf("chmod builder overlay: %v", err)
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
package_update: false
package_upgrade: false
users:
  - name: vmlite
    groups: [sudo]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: true
    ssh_authorized_keys:
      - %s
ssh_pwauth: false
`, strings.TrimSpace(string(pub)))
	if err := os.WriteFile(userData, []byte(userDataContent), 0o644); err != nil {
		t.Fatalf("write user-data: %v", err)
	}

	name := uniqueName()
	registerCleanup(t, systemURI, name)

	cfg := Config{
		Name:      name,
		Memory:    4096,
		CPUs:      4,
		OSVariant: "ubuntu24.04",
		Firmware:  firmwareBIOS,
		Source: Source{
			Type:      sourceCloudImage,
			Path:      stagedImage,
			CloudInit: userData,
		},
		Disk:     Disk{Size: 20, Path: overlay, Format: diskFormatQCOW2},
		Network:  Network{Mode: networkModeAuto},
		Graphics: Graphics{Type: graphicsNone},
	}
	configPath := writeConfig(t, cfg)

	out, err := runVmlite(t, systemURI, configPath)
	if err != nil {
		t.Fatalf("create builder VM: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created successfully") {
		t.Fatalf("unexpected builder output:\n%s", out)
	}

	xmlOut, err := virsh(t, systemURI, "dumpxml", name)
	if err != nil {
		t.Fatalf("builder dumpxml: %v\n%s", err, xmlOut)
	}
	if !strings.Contains(xmlOut, "<cpu mode='host-passthrough'") {
		t.Fatalf("builder does not use host-passthrough CPU:\n%s", xmlOut)
	}

	ip := waitForGuestSSH(t, systemURI, name, "vmlite", keyPath, 15*time.Minute)

	// Build once on the host and install the static binary through install.sh.
	binary := filepath.Join(workDir, "vmlite-linux-amd64")
	buildStaticVmlite(t, binary)
	if err := scpGuest(t, "vmlite", ip, keyPath, binary, "/tmp/vmlite"); err != nil {
		t.Fatalf("copy vmlite binary: %v", err)
	}

	installOut, err := sshGuest(t, "vmlite", ip, keyPath,
		"DEBIAN_FRONTEND=noninteractive sudo /tmp/vmlite install --binary /tmp/vmlite", 25*time.Minute)
	if err != nil {
		t.Fatalf("vmlite install in guest: %v\n%s", err, tailText(installOut, 4000))
	}

	verify := `set -e
command -v vmlite
test -e /dev/kvm
grep -Eq 'vmx|svm' /proc/cpuinfo
virt-install --version
virsh -c qemu:///session nodeinfo >/dev/null
echo SELFHOST_READY
`
	verifyOut, err := sshGuest(t, "vmlite", ip, keyPath, verify, 2*time.Minute)
	if err != nil {
		t.Fatalf("nested capability checks failed: %v\n%s", err, verifyOut)
	}
	if !strings.Contains(verifyOut, "SELFHOST_READY") {
		t.Fatalf("nested capability checks did not finish:\n%s", verifyOut)
	}

	nestedName := uniqueName()
	nestedDisk := "/tmp/nested-" + nestedName + ".qcow2"
	nestedCfg := Config{
		Name:      nestedName,
		Memory:    256,
		CPUs:      1,
		OSVariant: "debian12",
		Firmware:  firmwareBIOS,
		Source:    Source{Type: sourceNone},
		Disk:      Disk{Size: 1, Path: nestedDisk, Format: diskFormatQCOW2},
		Network:   Network{Mode: networkModeUser},
		Graphics:  Graphics{Type: graphicsNone},
	}
	nestedPath := writeConfig(t, nestedCfg)
	if err := scpGuest(t, "vmlite", ip, keyPath, nestedPath, "/tmp/vmlite-nested.json"); err != nil {
		t.Fatalf("copy nested config: %v", err)
	}

	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			return
		}
		cleanup := fmt.Sprintf(
			"virsh -c qemu:///session destroy %s 2>/dev/null || true; "+
				"virsh -c qemu:///session undefine %s --remove-all-storage 2>/dev/null || true; "+
				"rm -f /tmp/vmlite-nested.json %s",
			nestedName, nestedName, nestedDisk)
		_, _ = sshGuest(t, "vmlite", ip, keyPath, cleanup, time.Minute)
	})

	nestedOut, err := sshGuest(t, "vmlite", ip, keyPath,
		"/usr/local/bin/vmlite --connect qemu:///session /tmp/vmlite-nested.json", 2*time.Minute)
	if err != nil {
		t.Fatalf("nested vmlite create failed: %v\n%s", err, nestedOut)
	}
	if !strings.Contains(nestedOut, "created successfully") {
		t.Fatalf("nested vmlite output:\n%s", nestedOut)
	}

	state, err := sshGuest(t, "vmlite", ip, keyPath,
		"virsh -c qemu:///session domstate "+nestedName, time.Minute)
	if err != nil {
		t.Fatalf("nested domstate failed: %v\n%s", err, state)
	}
	if !strings.Contains(state, "running") {
		t.Fatalf("nested domain state = %q, want running", strings.TrimSpace(state))
	}

	t.Logf("vmlite installed in guest and created nested domain %s", nestedName)
}

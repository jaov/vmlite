//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProvisioners(t *testing.T) {
	if os.Getenv("VMLITE_TEST_PROVISION") != "1" {
		t.Skip("set VMLITE_TEST_PROVISION=1 to run shell provisioner tests")
	}
	image := os.Getenv("VMLITE_TEST_IMAGE")
	if image == "" {
		t.Skip("set VMLITE_TEST_IMAGE to an Ubuntu cloud image")
	}
	if _, err := os.Stat(image); err != nil {
		t.Skipf("VMLITE_TEST_IMAGE is not accessible: %v", err)
	}
	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt unavailable: %v", err)
	}

	workDir, err := os.MkdirTemp("/tmp", "vmlite-prov-")
	if err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	if err := os.Chmod(workDir, 0o755); err != nil {
		t.Fatalf("chmod work dir: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			t.Logf("VMLITE_TEST_KEEP=1: leaving provision work dir %s", workDir)
			return
		}
		pool := filepath.Base(workDir)
		_, _ = virsh(t, systemURI, "pool-destroy", pool)
		_, _ = virsh(t, systemURI, "pool-undefine", pool)
		os.RemoveAll(workDir)
	})
	if free := freeBytes(t, workDir); free < 3<<30 {
		t.Skipf("not enough tmpfs space in %s: %d bytes available", workDir, free)
	}

	staged := filepath.Join(workDir, "base.qcow2")
	stageImage(t, image, staged)
	overlay := filepath.Join(workDir, "overlay.qcow2")
	create := exec.Command("qemu-img", "create", "-f", "qcow2", "-F", "qcow2", "-b", staged, overlay, "5G")
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create overlay: %v: %s", err, out)
	}
	if err := os.Chmod(overlay, 0o666); err != nil {
		t.Fatalf("chmod overlay: %v", err)
	}

	name := uniqueName()
	registerCleanup(t, systemURI, name)
	cfg := Config{
		Name:      name,
		Memory:    1024,
		CPUs:      2,
		OSVariant: "ubuntu24.04",
		Firmware:  firmwareBIOS,
		Source:    Source{Type: sourceCloudImage, Path: staged},
		Disk:      Disk{Size: 5, Path: overlay, Format: diskFormatQCOW2},
		Network:   Network{Mode: networkModeDefault},
		Graphics:  Graphics{Type: graphicsNone},
		Provisioners: []Provisioner{
			{Type: "shell", Inline: "echo once > /var/tmp/vmlite-once\n", Run: "once"},
			{Type: "shell", Inline: "echo always >> /var/tmp/vmlite-always\n", Run: "always"},
		},
	}
	path := writeConfig(t, cfg)

	out, err := runVmlite(t, systemURI, path)
	if err != nil {
		t.Fatalf("up with provisioners failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created successfully") {
		t.Fatalf("unexpected up output:\n%s", out)
	}

	stateFile := filepath.Join(filepath.Dir(path), ".vmlite", name+".json")
	st, err := loadState(stateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st == nil || st.IP == "" || st.SSHKeyPath == "" {
		t.Fatalf("state missing IP/key: %+v", st)
	}

	check := func() string {
		t.Helper()
		result, err := sshGuest(t, "vmlite", st.IP, st.SSHKeyPath,
			"cat /var/tmp/vmlite-once; wc -l < /var/tmp/vmlite-always", 30*time.Second)
		if err != nil {
			t.Fatalf("SSH provisioner check failed: %v\n%s", err, result)
		}
		return result
	}

	first := check()
	if !strings.Contains(first, "once") || strings.Contains(first, "2") {
		t.Fatalf("unexpected initial provisioner output:\n%s", first)
	}

	out, err = runVmliteArgs(t, []string{"provision", path}, nil)
	if err != nil {
		t.Fatalf("provision failed: %v\n%s", err, out)
	}
	second := check()
	if strings.Count(second, "once") != 1 {
		t.Fatalf("once provisioner ran again:\n%s", second)
	}
	if !strings.Contains(second, "2") {
		t.Fatalf("always provisioner did not run again:\n%s", second)
	}

	statusOut, err := runVmliteArgs(t, []string{"status", path}, nil)
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, st.IP) {
		t.Fatalf("status did not report IP %s:\n%s", st.IP, statusOut)
	}
}

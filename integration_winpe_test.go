//go:build integration

package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vmlite/internal/plan"
)

func winpeHostOutput(t *testing.T, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	runner := &plan.LocalRunner{Stdout: io.Discard, Stderr: io.Discard}
	out, err := runner.Output(ctx, plan.Command{Name: name, Args: args})
	if err != nil {
		t.Fatalf("run %s %s: %v", name, strings.Join(args, " "), err)
	}
	return out
}

func waitForDomainShutoff(t *testing.T, uri, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		out, err := virsh(t, uri, "domstate", name)
		last = strings.TrimSpace(out)
		if err == nil && strings.Contains(last, "shut off") {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("domain %s did not shut off within %s (last %q)", name, timeout, last)
}

func TestWindowsPEBoot(t *testing.T) {
	if os.Getenv("VMLITE_TEST_WINPE") != "1" {
		t.Skip("set VMLITE_TEST_WINPE=1 to run the Windows PE test")
	}
	iso := os.Getenv("VMLITE_TEST_WINPE_ISO")
	if iso == "" {
		t.Skip("set VMLITE_TEST_WINPE_ISO to a custom WinPE ISO")
	}
	if _, err := os.Stat(iso); err != nil {
		t.Skipf("VMLITE_TEST_WINPE_ISO is not accessible: %v", err)
	}
	for _, bin := range []string{"mkfs.vfat", "7z"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is required for the WinPE marker check", bin)
		}
	}
	requireIntegration(t)
	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt unavailable: %v", err)
	}

	workDir, err := os.MkdirTemp("/tmp", "vmlite-winpe-")
	if err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	if err := os.Chmod(workDir, 0o755); err != nil {
		t.Fatalf("chmod work dir: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			t.Logf("VMLITE_TEST_KEEP=1: leaving WinPE work dir %s", workDir)
			return
		}
		os.RemoveAll(workDir)
	})
	if free := freeBytes(t, workDir); free < 2<<30 {
		t.Skipf("not enough tmpfs space in %s: %d bytes available", workDir, free)
	}

	stateISO := filepath.Join(workDir, "winpe.iso")
	stageImage(t, iso, stateISO)
	markerDisk := filepath.Join(workDir, "marker.img")
	winpeHostOutput(t, "qemu-img", "create", "-f", "raw", markerDisk, "64M")
	winpeHostOutput(t, "mkfs.vfat", "-n", "VMLITE", markerDisk)

	name := uniqueName()
	pool := filepath.Base(workDir)
	if _, err := virsh(t, systemURI, "pool-info", pool); err != nil {
		t.Cleanup(func() {
			if os.Getenv("VMLITE_TEST_KEEP") == "1" {
				return
			}
			_, _ = virsh(t, systemURI, "pool-destroy", pool)
			_, _ = virsh(t, systemURI, "pool-undefine", pool)
		})
	}
	registerCleanup(t, systemURI, name)

	cfg := Config{
		Name:     name,
		Memory:   1024,
		CPUs:     2,
		Firmware: firmwareBIOS,
		Source:   Source{Type: sourceISO, Path: stateISO},
		Disk:     Disk{Size: 1, Path: filepath.Join(workDir, "disk.qcow2"), Format: diskFormatQCOW2},
		Network:  Network{Mode: networkModeDefault},
		Graphics: Graphics{Type: graphicsVNC, Listen: "127.0.0.1"},
		ExtraDisks: []ExtraDisk{
			{Path: markerDisk, Device: "disk", Format: diskFormatRaw},
		},
	}
	configPath := writeConfig(t, cfg)

	out, err := runVmlite(t, systemURI, configPath)
	if err != nil {
		t.Fatalf("up failed: %v\n%s", err, out)
	}
	blk, err := virsh(t, systemURI, "domblklist", name, "--details")
	if err != nil {
		t.Fatalf("domblklist: %v", err)
	}
	if !strings.Contains(blk, "cdrom") {
		t.Fatalf("WinPE domain has no CD-ROM:\n%s", blk)
	}
	waitForDomainShutoff(t, systemURI, name, 10*time.Minute)

	extractDir := filepath.Join(workDir, "marker")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatalf("create marker dir: %v", err)
	}
	winpeHostOutput(t, "7z", "e", "-y", "-o"+extractDir, markerDisk, "vmlite-winpe-ok.txt")
	data, err := os.ReadFile(filepath.Join(extractDir, "vmlite-winpe-ok.txt"))
	if err != nil {
		t.Fatalf("read WinPE marker: %v", err)
	}
	if strings.TrimSpace(string(data)) != "winpe-ok" {
		t.Fatalf("marker = %q", data)
	}
}

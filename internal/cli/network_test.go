package cli

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveNetworkMAC(t *testing.T) {
	cfg := &Config{Name: "dev", Network: Network{MAC: "52:54:00:AA:BB:CC"}}
	if err := resolveNetworkMAC(cfg); err != nil {
		t.Fatalf("resolveNetworkMAC explicit: %v", err)
	}
	if cfg.NetworkMAC != "52:54:00:aa:bb:cc" {
		t.Fatalf("mac = %q", cfg.NetworkMAC)
	}

	cfg = &Config{Name: "dev", Network: Network{IP: "192.168.122.50"}}
	if err := resolveNetworkMAC(cfg); err != nil {
		t.Fatalf("resolveNetworkMAC derived: %v", err)
	}
	hw, err := net.ParseMAC(cfg.NetworkMAC)
	if err != nil || len(hw) != 6 || !strings.HasPrefix(cfg.NetworkMAC, "52:54:00:") {
		t.Fatalf("derived mac = %q", cfg.NetworkMAC)
	}

	other := &Config{Name: "other", Network: Network{IP: "192.168.122.51"}}
	if err := resolveNetworkMAC(other); err != nil {
		t.Fatalf("resolveNetworkMAC other: %v", err)
	}
	if other.NetworkMAC == cfg.NetworkMAC {
		t.Fatalf("different names got the same MAC %q", cfg.NetworkMAC)
	}
}

func TestEnsureDHCPReservation(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "virsh.log")
	fakeVirsh(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+logPath+"\nexit 0\n")

	cfg := &Config{
		Name:       "cmd-test",
		Network:    Network{IP: "192.168.122.50"},
		NetworkMAC: "52:54:00:aa:bb:cc",
	}
	if err := ensureDHCPReservation(cfg, systemURI, false); err != nil {
		t.Fatalf("ensureDHCPReservation: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read virsh log: %v", err)
	}
	for _, want := range []string{"net-update", "default", "add", "ip-dhcp-host", "<host mac='52:54:00:aa:bb:cc' ip='192.168.122.50'/>"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("virsh log missing %q:\n%s", want, data)
		}
	}
}

func TestEnsureDHCPReservationDryRun(t *testing.T) {
	cfg := &Config{
		Name:       "cmd-test",
		Network:    Network{IP: "192.168.122.50"},
		NetworkMAC: "52:54:00:aa:bb:cc",
	}
	out, err := captureCommandOutput(t, func() error {
		return ensureDHCPReservation(cfg, systemURI, true)
	})
	if err != nil {
		t.Fatalf("ensureDHCPReservation dry-run: %v", err)
	}
	if !strings.Contains(out, "Would reserve IP 192.168.122.50") {
		t.Fatalf("output = %q", out)
	}
}

func TestRemoveDHCPReservation(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "virsh.log")
	fakeVirsh(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+logPath+"\nexit 0\n")

	if err := removeDHCPReservation(systemURI, "default", "52:54:00:aa:bb:cc", "192.168.122.50"); err != nil {
		t.Fatalf("removeDHCPReservation: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read virsh log: %v", err)
	}
	for _, want := range []string{"net-update", "default", "delete", "ip-dhcp-host"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("virsh log missing %q:\n%s", want, data)
		}
	}
}

func TestEnsureDHCPReservationRejectsSession(t *testing.T) {
	cfg := &Config{
		Name:       "cmd-test",
		Network:    Network{IP: "192.168.122.50"},
		NetworkMAC: "52:54:00:aa:bb:cc",
	}
	err := ensureDHCPReservation(cfg, sessionURI, false)
	if err == nil || !strings.Contains(err.Error(), "qemu:///system") {
		t.Fatalf("ensureDHCPReservation = %v, want session error", err)
	}
}

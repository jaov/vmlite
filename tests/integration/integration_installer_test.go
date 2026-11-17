//go:build integration

package integration

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func installerISO(t *testing.T, env string) string {
	t.Helper()

	iso := os.Getenv(env)
	if iso == "" {
		t.Skipf("set %s to an installer ISO", env)
	}
	if _, err := os.Stat(iso); err != nil {
		t.Skipf("%s is not accessible: %v", env, err)
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("/dev/kvm is not available")
	}
	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt is unavailable: %v", err)
	}
	return iso
}

func waitForDomainRunning(t *testing.T, uri, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		out, err := virsh(t, uri, "domstate", name)
		last = strings.TrimSpace(out)
		if err == nil && last == "running" {
			return
		}
		if last == "shut off" || last == "crashed" {
			t.Fatalf("domain %s is %s", name, last)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("domain %s did not reach running (last state %q)", name, last)
}

func waitForVNC(t *testing.T, uri, name string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		out, err := virsh(t, uri, "domdisplay", name)
		if err == nil {
			last = strings.TrimSpace(out)
			if strings.HasPrefix(last, "vnc://") {
				hostPort := strings.TrimPrefix(last, "vnc://")
				host, portText, splitErr := net.SplitHostPort(hostPort)
				if splitErr == nil {
					port, convErr := strconv.Atoi(portText)
					if convErr == nil {
						// domdisplay reports the VNC display number (:0),
						// not the TCP port (5900 + display).
						if port < 5900 {
							port += 5900
						}
						if host == "" {
							host = "127.0.0.1"
						}
						conn, dialErr := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 2*time.Second)
						if dialErr == nil {
							conn.Close()
							return last
						}
					}
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("VNC display for %s was not reachable within %s (last %q)", name, timeout, last)
	return ""
}

func testInstallerBoot(t *testing.T, distro, env string) {
	t.Helper()
	iso := installerISO(t, env)

	// virt-install creates a transient directory pool for the ISO directory.
	// Remove it unless it existed before this test.
	isoPool := filepath.Base(filepath.Dir(iso))
	_, poolErr := virsh(t, systemURI, "pool-info", isoPool)
	poolExisted := poolErr == nil
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" || poolExisted {
			return
		}
		_, _ = virsh(t, systemURI, "pool-destroy", isoPool)
		_, _ = virsh(t, systemURI, "pool-undefine", isoPool)
	})

	name := uniqueName()
	registerCleanup(t, systemURI, name)

	cfg := Config{
		Name:     name,
		Memory:   1024,
		CPUs:     1,
		Firmware: firmwareBIOS,
		Source:   Source{Type: sourceISO, Path: iso},
		Disk:     Disk{Size: 5},
		Network:  Network{Mode: networkModeAuto},
		Graphics: Graphics{Type: graphicsVNC, Listen: "127.0.0.1"},
	}
	configPath := writeConfig(t, cfg)

	out, err := runVmlite(t, systemURI, configPath)
	if err != nil {
		t.Fatalf("vmlite create failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created successfully") {
		t.Fatalf("unexpected vmlite output:\n%s", out)
	}

	waitForDomainRunning(t, systemURI, name, 60*time.Second)

	xmlOut, err := virsh(t, systemURI, "dumpxml", name)
	if err != nil {
		t.Fatalf("dumpxml: %v\n%s", err, xmlOut)
	}
	for _, want := range []string{
		"device='cdrom'",
		"type='vnc'",
		"listen='127.0.0.1'",
		iso,
	} {
		if !strings.Contains(xmlOut, want) {
			t.Errorf("%s installer XML missing %q:\n%s", distro, want, xmlOut)
		}
	}

	display := waitForVNC(t, systemURI, name, 90*time.Second)
	t.Logf("%s installer VNC reachable at %s", distro, display)
}

func TestOpenBSDInstallerBoot(t *testing.T) {
	testInstallerBoot(t, "openbsd", "VMLITE_TEST_OPENBSD_ISO")
}

func TestNetBSDInstallerBoot(t *testing.T) {
	testInstallerBoot(t, "netbsd", "VMLITE_TEST_NETBSD_ISO")
}

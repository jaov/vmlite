// Package host checks the local KVM/libvirt runtime and connection.
package host

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

const (
	SystemURI  = "qemu:///system"
	SessionURI = "qemu:///session"
)

// Check verifies that the tools and hardware needed to run KVM guests exist.
func Check() error {
	for _, bin := range []string{"virt-install", "virsh"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("required binary %q not found in PATH: %w", bin, err)
		}
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return fmt.Errorf("/dev/kvm is not available (%v): hardware virtualization may be disabled in firmware or the kvm kernel modules are not loaded", err)
	}
	return nil
}

// ValidateConnection checks the host and verifies that uri is reachable.
func ValidateConnection(uri string) error {
	if err := Check(); err != nil {
		return err
	}
	if !reachable(uri) {
		return fmt.Errorf("cannot connect to libvirt at %s", uri)
	}
	return nil
}

// Preflight verifies the host and returns the preferred libvirt URI.
func Preflight() (string, error) {
	if err := Check(); err != nil {
		return "", err
	}
	if reachable(SystemURI) {
		return SystemURI, nil
	}
	if reachable(SessionURI) {
		fmt.Fprintln(os.Stderr, "warning: qemu:///system is not accessible; falling back to qemu:///session (user networking, per-user storage, and stricter mount permissions apply)")
		return SessionURI, nil
	}
	return "", errors.New("no accessible libvirt daemon: both " + SystemURI + " and " + SessionURI + " failed to respond to 'virsh nodeinfo'")
}

func reachable(uri string) bool {
	cmd := exec.Command("virsh", "-c", uri, "nodeinfo")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

const (
	systemURI  = "qemu:///system"
	sessionURI = "qemu:///session"
)

// CheckHost verifies that the tools and hardware needed to run KVM guests are
// present on this host.
func CheckHost() error {
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

// ValidateConnection checks that the host is ready and that the given libvirt
// URI is reachable. It performs no fallback.
func ValidateConnection(uri string) error {
	if err := CheckHost(); err != nil {
		return err
	}
	if !daemonReachable(uri) {
		return fmt.Errorf("cannot connect to libvirt at %s", uri)
	}
	return nil
}

// RunPreflightChecks verifies that the host is ready to run KVM guests and
// returns the libvirt connection URI that should be used.
func RunPreflightChecks() (string, error) {
	if err := CheckHost(); err != nil {
		return "", err
	}

	if daemonReachable(systemURI) {
		return systemURI, nil
	}

	if daemonReachable(sessionURI) {
		fmt.Fprintln(os.Stderr, "warning: qemu:///system is not accessible; falling back to qemu:///session (user networking, per-user storage, and stricter mount permissions apply)")
		return sessionURI, nil
	}

	return "", errors.New("no accessible libvirt daemon: both " + systemURI + " and " + sessionURI + " failed to respond to 'virsh nodeinfo'")
}

// daemonReachable reports whether virsh can talk to the daemon behind uri.
func daemonReachable(uri string) bool {
	cmd := exec.Command("virsh", "-c", uri, "nodeinfo")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

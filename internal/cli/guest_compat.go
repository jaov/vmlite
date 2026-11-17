package cli

import (
	"time"

	"vmlite/internal/guest"
)

// Compatibility wrappers while the root package is split into internal
// packages. New code should use internal/guest directly.
type guestTarget = guest.Target

func resolveGuestTarget(configPath, stateDir, user string, timeout time.Duration) (*guestTarget, error) {
	return guest.ResolveTarget(configPath, stateDir, user, timeout)
}

func waitForGuestSSH(target *guestTarget, timeout time.Duration) error {
	return guest.WaitForSSH(target, timeout)
}

func sshOptionArgs(target *guestTarget, tty bool) []string {
	return guest.SSHOptionArgs(target, tty)
}

func stdinIsTerminal() bool {
	return guest.StdinIsTerminal()
}

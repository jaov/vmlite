package main

import (
	"context"
	"time"

	"vmlite/internal/plan"
	"vmlite/internal/provision"
)

// Compatibility wrappers while the root package is split into internal
// packages. New code should use internal/provision directly.
func ensureSSHKey(cfg *Config, layout projectLayout) error {
	return provision.EnsureSSHKey(cfg, layout)
}

func provisionVM(cfg *Config, st *State, layout projectLayout, force, dryRun bool, timeout time.Duration) error {
	return provision.ProvisionVM(cfg, st, layout, force, dryRun, timeout)
}

func waitForIPv4(uri, name string, timeout time.Duration) (string, error) {
	return provision.WaitForIPv4(uri, name, timeout)
}

func waitForSSH(ctx context.Context, runner *plan.SSHRunner, timeout time.Duration) error {
	return provision.WaitForSSH(ctx, runner, timeout)
}

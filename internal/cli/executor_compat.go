package cli

import (
	guestinstall "vmlite/internal/installer"
	"vmlite/internal/vm"
)

// Compatibility wrappers while the root package is split into internal
// packages. New code should use internal/vm directly.
func buildVirtInstallArgs(cfg *Config, uri string) ([]string, error) {
	return vm.BuildVirtInstallArgs(cfg, uri)
}

func buildSourceArgs(cfg *Config, spec guestinstall.Spec) ([]string, error) {
	return vm.BuildSourceArgs(cfg, spec)
}

func bootArgs(firmware string, order []string, secureBoot bool) []string {
	return vm.BootArgs(firmware, order, secureBoot)
}

func configureInstallerLifecycle(cfg *Config, uri string, spec guestinstall.Spec) error {
	return vm.ConfigureInstallerLifecycle(cfg, uri, spec)
}

func RunVirtInstall(cfg *Config, uri string) error {
	return vm.RunVirtInstall(cfg, uri)
}

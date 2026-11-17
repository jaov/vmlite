package cli

import (
	guestinstall "vmlite/internal/installer"
	"vmlite/internal/media"
)

// Compatibility wrappers while the root package is split into internal
// packages. New code should use internal/media directly.
var errImageWouldDownload = media.ErrImageWouldDownload

func resolveImage(cfg *Config, offline, dryRun bool) (string, error) {
	return media.ResolveImage(cfg, offline, dryRun)
}

func extraDiskFormat(d ExtraDisk) (string, error) {
	return media.ExtraDiskFormat(d)
}

func prepareOverlay(cfg *Config) (string, error) {
	return media.PrepareOverlay(cfg)
}

func prepareSeed(cfg *Config) (string, error) {
	return media.PrepareSeed(cfg)
}

func CheckImageTools(cfg *Config) error {
	return media.CheckImageTools(cfg)
}

func prepareInstallerISO(cfg *Config, spec guestinstall.Spec) (string, error) {
	return media.PrepareInstallerISO(cfg, spec)
}

func prepareInstallerKernel(cfg *Config, spec guestinstall.Spec) (string, string, error) {
	return media.PrepareInstallerKernel(cfg, spec)
}

func prepareInstallerSeed(cfg *Config) (string, error) {
	return media.PrepareInstallerSeed(cfg)
}

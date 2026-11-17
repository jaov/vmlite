package main

import "vmlite/internal/libvirt"

// Compatibility wrappers while the root package is split into internal
// packages. New code should call internal/libvirt directly.
func runVirsh(uri string, args ...string) (string, error) {
	return libvirt.Run(uri, args...)
}

func domainStatus(uri, name string) (string, bool) {
	return libvirt.Status(uri, name)
}

func domainUUID(uri, name string) string {
	return libvirt.UUID(uri, name)
}

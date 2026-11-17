package cli

import "vmlite/internal/host"

// Compatibility wrappers while the root package is split into internal
// packages. New code should use internal/host directly.
const (
	systemURI  = host.SystemURI
	sessionURI = host.SessionURI
)

func CheckHost() error {
	return host.Check()
}

func ValidateConnection(uri string) error {
	return host.ValidateConnection(uri)
}

func RunPreflightChecks() (string, error) {
	return host.Preflight()
}

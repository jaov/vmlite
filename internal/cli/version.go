package cli

import (
	"fmt"
	"runtime/debug"
)

// Version is set by the root main from main.version.
var Version = "dev"

func versionString() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	if Version == "" {
		return "dev"
	}
	return Version
}

func runVersion(args []string) error {
	if len(args) > 0 {
		return errUsage
	}
	fmt.Printf("vmlite %s\n", versionString())
	return nil
}

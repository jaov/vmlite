package main

import (
	"fmt"
	"runtime/debug"
)

// version is overridden with -ldflags "-X main.version=<tag>".
var version = "dev"

func versionString() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	if version == "" {
		return "dev"
	}
	return version
}

func runVersion(args []string) error {
	if len(args) > 0 {
		return errUsage
	}
	fmt.Printf("vmlite %s\n", versionString())
	return nil
}

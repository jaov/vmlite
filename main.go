package main

import (
	"errors"
	"fmt"
	"os"
)

var errUsage = errors.New("usage: vmlite [up|provision|status|destroy|validate|install|doctor] [options] [config.json]")

func main() {
	err := dispatch(os.Args[1:])
	if err == nil {
		return
	}
	if errors.Is(err, errNotRunning) {
		os.Exit(3)
	}
	fmt.Fprintf(os.Stderr, "vmlite: %v\n", err)
	if errors.Is(err, errUsage) {
		os.Exit(2)
	}
	os.Exit(1)
}

func dispatch(args []string) error {
	if len(args) == 0 {
		return runUp(nil)
	}
	switch args[0] {
	case "up":
		return runUp(args[1:])
	case "provision":
		return runProvision(args[1:])
	case "status":
		return runStatus(args[1:])
	case "destroy":
		return runDestroy(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "install":
		return runInstall(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	default:
		// Preserve the original `vmlite [config]` form as an implicit up.
		return runUp(args)
	}
}

// run is the original entrypoint used by unit tests; it is an alias for up.
func run(args []string) error {
	return runUp(args)
}

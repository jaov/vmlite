package main

import (
	"errors"
	"fmt"
	"os"
)

var errUsage = errors.New("usage: vmlite [up|provision|status|start|stop|restart|ssh|scp|console|list|destroy|validate|install|doctor|version|help] [options] [config.json]")

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
	if args[0] == "help" {
		return runHelp(args[1:])
	}
	if args[0] == "-h" || args[0] == "--help" {
		return runHelp(nil)
	}
	if args[0] == "--version" || args[0] == "-version" {
		return runVersion(args[1:])
	}
	if hasHelpFlag(args) {
		if cmd, ok := lookupCommand(args[0]); ok {
			return runHelp([]string{cmd.Name})
		}
		return runHelp([]string{"up"})
	}
	switch args[0] {
	case "up":
		return runUp(args[1:])
	case "provision":
		return runProvision(args[1:])
	case "status":
		return runStatus(args[1:])
	case "start":
		return runStart(args[1:])
	case "stop":
		return runStop(args[1:])
	case "restart":
		return runRestart(args[1:])
	case "ssh":
		return runSSH(args[1:])
	case "scp":
		return runSCP(args[1:])
	case "console":
		return runConsole(args[1:])
	case "list":
		return runList(args[1:])
	case "destroy":
		return runDestroy(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "install":
		return runInstall(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "version":
		return runVersion(args[1:])
	default:
		// Preserve the original `vmlite [config]` form as an implicit up.
		return runUp(args)
	}
}

// run is the original entrypoint used by unit tests; it is an alias for up.
func run(args []string) error {
	return runUp(args)
}

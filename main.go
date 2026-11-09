package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

var errUsage = errors.New("usage: vmlite [--connect URI] [config.json]")

func main() {
	err := run(os.Args[1:])
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "vmlite: %v\n", err)
	if errors.Is(err, errUsage) {
		os.Exit(2)
	}
	os.Exit(1)
}

func run(args []string) error {
	fs := flag.NewFlagSet("vmlite", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	connect := fs.String("connect", "", "libvirt connection URI (default: auto-detect)")
	if err := fs.Parse(args); err != nil {
		return errUsage
	}

	rest := fs.Args()
	if len(rest) > 1 {
		return errUsage
	}
	configPath := "vm.json"
	if len(rest) == 1 {
		configPath = rest[0]
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	// An explicit --connect flag wins over VMLITE_CONNECT, which in turn wins
	// over automatic detection.
	uri := *connect
	if uri == "" {
		uri = os.Getenv("VMLITE_CONNECT")
	}
	if uri != "" {
		if err := ValidateConnection(uri); err != nil {
			return err
		}
	} else {
		uri, err = RunPreflightChecks()
		if err != nil {
			return err
		}
	}
	fmt.Printf("Using libvirt connection: %s\n", uri)

	if err := RunVirtInstall(cfg, uri); err != nil {
		return err
	}

	fmt.Printf("VM %q created successfully via %s\n", cfg.Name, uri)
	return nil
}

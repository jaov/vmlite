package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"vmlite/internal/install"
	"vmlite/internal/plan"
)

func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prefix := fs.String("prefix", "/usr/local/bin", "install destination directory")
	binary := fs.String("binary", "", "install a local vmlite binary")
	binaryURL := fs.String("binary-url", "", "download a vmlite binary from this URL")
	binarySHA := fs.String("binary-sha256", "", "SHA-256 for --binary-url")
	noDeps := fs.Bool("no-deps", false, "skip host package installation")
	dryRun := fs.Bool("dry-run", false, "print actions without changing the host")
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	if fs.NArg() > 0 {
		return errUsage
	}

	p, err := install.DetectPlatform()
	if err != nil {
		return err
	}
	steps, err := install.BuildPlan(p, install.Options{
		Prefix:       *prefix,
		NoDeps:       *noDeps,
		BinaryPath:   *binary,
		BinaryURL:    *binaryURL,
		BinarySHA256: *binarySHA,
	})
	if err != nil {
		return err
	}

	runner := &plan.LocalRunner{DryRun: *dryRun, Stdout: os.Stdout, Stderr: os.Stderr}
	if *dryRun {
		fmt.Printf("Dry run for %s (%s, %s)\n", p.ID, p.Family, p.Arch)
	}
	if err := steps.Run(context.Background(), runner, os.Stdout); err != nil {
		return err
	}
	if !*dryRun {
		fmt.Printf("vmlite installed at %s\n", filepath.Join(*prefix, "vmlite"))
	}
	return nil
}

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prefix := fs.String("prefix", "/usr/local/bin", "vmlite install destination")
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	if fs.NArg() > 0 {
		return errUsage
	}
	p, err := install.DetectPlatform()
	if err != nil {
		return err
	}
	return install.Doctor(p, *prefix)
}

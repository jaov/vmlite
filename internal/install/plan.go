package install

import (
	"fmt"
	"os"
	"path/filepath"

	"vmlite/internal/plan"
)

type Options struct {
	Prefix       string
	NoDeps       bool
	BinaryPath   string
	BinaryURL    string
	BinarySHA256 string
}

// BuildPlan constructs the host installation plan.
func BuildPlan(p Platform, opts Options) (plan.Plan, error) {
	if opts.Prefix == "" {
		opts.Prefix = "/usr/local/bin"
	}
	if opts.BinaryPath != "" && opts.BinaryURL != "" {
		return nil, fmt.Errorf("--binary and --binary-url are mutually exclusive")
	}

	var steps plan.Plan

	if !opts.NoDeps {
		manager, err := NewManager(p)
		if err != nil {
			return nil, err
		}
		packages := PackagesFor(p.Family)
		if len(packages) > 0 {
			steps = append(steps, manager.PackageStep(packages))
		}
	}

	steps = append(steps,
		VerifyStep(BinariesFor()),
		KVMStep(),
		GroupsStep(),
		DefaultNetworkStep(),
	)

	dest := filepath.Join(opts.Prefix, "vmlite")
	switch {
	case opts.BinaryURL != "":
		if !validSHA(opts.BinarySHA256) {
			return nil, fmt.Errorf("--binary-url requires --binary-sha256 (64 hex characters)")
		}
		steps = append(steps, BlobStep{
			Blob: Blob{
				URL:    opts.BinaryURL,
				SHA256: opts.BinarySHA256,
				Dest:   dest,
			},
			Description: "download and install vmlite",
		})
	case opts.BinaryPath != "":
		steps = append(steps, BinaryStep{Source: opts.BinaryPath, Dest: dest})
	default:
		self, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate current executable: %w", err)
		}
		steps = append(steps, BinaryStep{Source: self, Dest: dest})
	}

	return steps, nil
}

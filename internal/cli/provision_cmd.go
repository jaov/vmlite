package cli

import (
	"flag"
	"fmt"
	"io"
	"time"

	"vmlite/internal/provision"
)

func newStateFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parseConfigArgs(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", errUsage
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return "", errUsage
	}
	if len(rest) == 1 {
		return rest[0], nil
	}
	return "vm.json", nil
}

func provisionFlagSet() (*flag.FlagSet, *string, *bool, *bool, *time.Duration) {
	fs := newStateFlagSet("provision")
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	force := fs.Bool("force", false, "rerun once-provisioners")
	dryRun := fs.Bool("dry-run", false, "print actions without changing anything")
	timeout := fs.Duration("timeout", 0, "maximum wait for guest readiness (default: 5m, or 30m for SSH-completing installers)")
	return fs, stateDir, force, dryRun, timeout
}

func runProvision(args []string) error {
	fs, stateDir, force, dryRun, timeout := provisionFlagSet()
	configPath, err := parseConfigArgs(fs, args)
	if err != nil {
		return err
	}
	if *timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	layout, err := newProjectLayout(configPath, *stateDir, cfg.Name)
	if err != nil {
		return err
	}
	lock, err := acquireProjectLock(layout)
	if err != nil {
		return err
	}
	defer lock.Close()
	st, err := loadState(layout.StateFile)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("no state for %q at %s", cfg.Name, layout.StateFile)
	}
	return provision.ProvisionVM(cfg, st, layout, *force, *dryRun, *timeout)
}

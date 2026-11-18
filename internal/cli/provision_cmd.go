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

func parsePositional(fs *flag.FlagSet, args []string) (string, error) {
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
	return "", nil
}

func provisionFlagSet() (*flag.FlagSet, *string, *bool, *bool, *time.Duration) {
	fs := newStateFlagSet("provision")
	stateDir := fs.String("state-dir", "", stateDirFlagHelp)
	force := fs.Bool("force", false, "rerun once-provisioners")
	dryRun := fs.Bool("dry-run", false, "print actions without changing anything")
	timeout := fs.Duration("timeout", 0, "maximum wait for guest readiness (default: 5m, or 30m for SSH-completing installers)")
	return fs, stateDir, force, dryRun, timeout
}

func runProvision(args []string) error {
	fs, stateDir, force, dryRun, timeout := provisionFlagSet()
	target, err := parsePositional(fs, args)
	if err != nil {
		return err
	}
	if *timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}

	t, err := resolveVM(target, *stateDir)
	if err != nil {
		return err
	}
	if t.State == nil {
		return fmt.Errorf("no state for %q at %s", t.Config.Name, t.Layout.StateFile)
	}
	if t.Config == nil {
		return fmt.Errorf("config %q is required to provision", t.Layout.ConfigPath)
	}
	lock, err := acquireProjectLock(t.Layout)
	if err != nil {
		return err
	}
	defer lock.Close()
	return provision.ProvisionVM(t.Config, t.State, t.Layout, *force, *dryRun, *timeout)
}

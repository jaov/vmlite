package cli

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

func powerStateFlagSet(name string) (*flag.FlagSet, *string, *bool) {
	fs := newStateFlagSet(name)
	stateDir := fs.String("state-dir", "", stateDirFlagHelp)
	dryRun := fs.Bool("dry-run", false, "print actions without changing anything")
	return fs, stateDir, dryRun
}

func loadPowerState(target, stateDir string) (projectLayout, *State, *projectLock, error) {
	t, err := resolveVM(target, stateDir)
	if err != nil {
		return projectLayout{}, nil, nil, err
	}
	if t.State == nil {
		name := target
		if t.Config != nil {
			name = t.Config.Name
		}
		return projectLayout{}, nil, nil, fmt.Errorf("no state for %q at %s", name, t.Layout.StateFile)
	}
	lock, err := acquireProjectLock(t.Layout)
	if err != nil {
		return projectLayout{}, nil, nil, err
	}
	return t.Layout, t.State, lock, nil
}

func startFlagSet() (*flag.FlagSet, *string, *bool) {
	return powerStateFlagSet("start")
}

func runStart(args []string) error {
	fs, stateDir, dryRun := startFlagSet()
	target, err := parsePositional(fs, args)
	if err != nil {
		return err
	}
	layout, st, lock, err := loadPowerState(target, *stateDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	text, exists := domainStatus(st.LibvirtURI, st.Name)
	if !exists {
		return fmt.Errorf("VM %q is missing", st.Name)
	}
	if strings.Contains(text, "running") {
		fmt.Printf("VM %q is already running\n", st.Name)
		return nil
	}
	if !strings.Contains(text, "shut off") {
		return fmt.Errorf("VM %q is %q; expected running or shut off", st.Name, text)
	}
	if *dryRun {
		fmt.Printf("Would start VM %q\n", st.Name)
		return nil
	}
	if out, err := runVirsh(st.LibvirtURI, "start", st.Name); err != nil {
		return fmt.Errorf("start VM %q: %w: %s", st.Name, err, out)
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := saveState(layout.StateFile, st); err != nil {
		return fmt.Errorf("VM %q started but state update failed: %w", st.Name, err)
	}
	fmt.Printf("VM %q started\n", st.Name)
	return nil
}

func stopFlagSet() (*flag.FlagSet, *string, *time.Duration, *bool, *bool) {
	fs := newStateFlagSet("stop")
	stateDir := fs.String("state-dir", "", stateDirFlagHelp)
	timeout := fs.Duration("timeout", 2*time.Minute, "wait for graceful shutdown before failing")
	force := fs.Bool("force", false, "destroy the VM if graceful shutdown times out")
	dryRun := fs.Bool("dry-run", false, "print actions without changing anything")
	return fs, stateDir, timeout, force, dryRun
}

func runStop(args []string) error {
	fs, stateDir, timeout, force, dryRun := stopFlagSet()
	target, err := parsePositional(fs, args)
	if err != nil {
		return err
	}
	layout, st, lock, err := loadPowerState(target, *stateDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	text, exists := domainStatus(st.LibvirtURI, st.Name)
	if !exists {
		return fmt.Errorf("VM %q is missing", st.Name)
	}
	if !strings.Contains(text, "running") {
		fmt.Printf("VM %q is already stopped\n", st.Name)
		return nil
	}
	if *dryRun {
		fmt.Printf("Would shut down VM %q\n", st.Name)
		return nil
	}
	if out, err := runVirsh(st.LibvirtURI, "shutdown", st.Name); err != nil {
		return fmt.Errorf("shutdown VM %q: %w: %s", st.Name, err, out)
	}
	if err := waitForPowerOff(st.LibvirtURI, st.Name, *timeout); err != nil {
		if !*force {
			return fmt.Errorf("%w; use --force to destroy it", err)
		}
		if out, err := runVirsh(st.LibvirtURI, "destroy", st.Name); err != nil {
			return fmt.Errorf("force stop VM %q: %w: %s", st.Name, err, out)
		}
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := saveState(layout.StateFile, st); err != nil {
		return fmt.Errorf("VM %q stopped but state update failed: %w", st.Name, err)
	}
	fmt.Printf("VM %q stopped\n", st.Name)
	return nil
}

func restartFlagSet() (*flag.FlagSet, *string, *bool) {
	return powerStateFlagSet("restart")
}

func runRestart(args []string) error {
	fs, stateDir, dryRun := restartFlagSet()
	target, err := parsePositional(fs, args)
	if err != nil {
		return err
	}
	layout, st, lock, err := loadPowerState(target, *stateDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	text, exists := domainStatus(st.LibvirtURI, st.Name)
	if !exists {
		return fmt.Errorf("VM %q is missing", st.Name)
	}
	if strings.Contains(text, "shut off") {
		if *dryRun {
			fmt.Printf("Would start VM %q\n", st.Name)
			return nil
		}
		if out, err := runVirsh(st.LibvirtURI, "start", st.Name); err != nil {
			return fmt.Errorf("start VM %q: %w: %s", st.Name, err, out)
		}
	} else if strings.Contains(text, "running") {
		if *dryRun {
			fmt.Printf("Would reboot VM %q\n", st.Name)
			return nil
		}
		if out, err := runVirsh(st.LibvirtURI, "reboot", st.Name); err != nil {
			return fmt.Errorf("reboot VM %q: %w: %s", st.Name, err, out)
		}
	} else {
		return fmt.Errorf("VM %q is %q; expected running or shut off", st.Name, text)
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := saveState(layout.StateFile, st); err != nil {
		return fmt.Errorf("VM %q restarted but state update failed: %w", st.Name, err)
	}
	fmt.Printf("VM %q restarted\n", st.Name)
	return nil
}

func waitForPowerOff(uri, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		text, exists := domainStatus(uri, name)
		last = text
		if !exists {
			return fmt.Errorf("VM %q disappeared", name)
		}
		if strings.Contains(text, "shut off") {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("VM %q did not shut down within %s (last state %q)", name, timeout, last)
}

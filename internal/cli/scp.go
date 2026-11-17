package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func scpFlagSet() (*flag.FlagSet, *string, *string, *time.Duration, *bool, *bool) {
	fs := newStateFlagSet("scp")
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	user := fs.String("user", "", "SSH user override")
	timeout := fs.Duration("timeout", 30*time.Second, "maximum wait for guest IP/SSH readiness")
	recursive := fs.Bool("recursive", false, "copy directories recursively")
	dryRun := fs.Bool("dry-run", false, "print the scp command without running it")
	return fs, stateDir, user, timeout, recursive, dryRun
}

func runSCP(args []string) error {
	fs, stateDir, user, timeout, recursive, dryRun := scpFlagSet()
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	rest := fs.Args()
	if len(rest) < 2 || len(rest) > 3 {
		return errUsage
	}
	local := rest[0]
	remote := rest[1]
	configPath := "vm.json"
	if len(rest) == 3 {
		configPath = rest[2]
	}
	if _, err := os.Stat(local); err != nil {
		return fmt.Errorf("local path %q is not accessible: %w", local, err)
	}
	local, err := filepath.Abs(local)
	if err != nil {
		return fmt.Errorf("resolve local path %q: %w", local, err)
	}

	target, err := resolveGuestTarget(configPath, *stateDir, *user, *timeout)
	if err != nil {
		return err
	}
	if !*dryRun {
		if err := waitForGuestSSH(target, *timeout); err != nil {
			return err
		}
	}

	scpArgs := sshOptionArgs(target, false)
	if *recursive {
		scpArgs = append(scpArgs, "-r")
	}
	scpArgs = append(scpArgs, local, target.User+"@"+target.IP+":"+remote)
	if *dryRun {
		fmt.Printf("+ scp %s\n", strings.Join(scpArgs, " "))
		return nil
	}

	cmd := exec.Command("scp", scpArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp %s to %s@%s:%s: %w", local, target.User, target.IP, remote, err)
	}
	return nil
}

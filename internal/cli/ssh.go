package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func sshFlagSet() (*flag.FlagSet, *string, *string, *string, *time.Duration, *bool) {
	fs := newStateFlagSet("ssh")
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	user := fs.String("user", "", "SSH user override")
	command := fs.String("command", "", "run a non-interactive remote command")
	timeout := fs.Duration("timeout", 30*time.Second, "maximum wait for guest IP/SSH readiness")
	dryRun := fs.Bool("dry-run", false, "print the ssh command without running it")
	return fs, stateDir, user, command, timeout, dryRun
}

func runSSH(args []string) error {
	fs, stateDir, user, command, timeout, dryRun := sshFlagSet()
	configPath, err := parseConfigArgs(fs, args)
	if err != nil {
		return err
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

	sshArgs := sshOptionArgs(target, *command == "" && stdinIsTerminal())
	sshArgs = append(sshArgs, target.User+"@"+target.IP)
	if *command != "" {
		sshArgs = append(sshArgs, *command)
	}
	if *dryRun {
		fmt.Printf("+ ssh %s\n", strings.Join(sshArgs, " "))
		return nil
	}

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh %s@%s: %w", target.User, target.IP, err)
	}
	return nil
}

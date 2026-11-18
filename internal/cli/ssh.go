package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func sshFlagSet() (*flag.FlagSet, *string, *string, *string, *time.Duration, *bool, *bool) {
	fs := newStateFlagSet("ssh")
	stateDir := fs.String("state-dir", "", stateDirFlagHelp)
	user := fs.String("user", "", "SSH user override")
	command := fs.String("command", "", "run a non-interactive remote command")
	timeout := fs.Duration("timeout", 30*time.Second, "maximum wait for guest IP/SSH readiness")
	dryRun := fs.Bool("dry-run", false, "print the ssh command without running it")
	printConfig := fs.Bool("print-config", false, "print an SSH config block instead of connecting")
	return fs, stateDir, user, command, timeout, dryRun, printConfig
}

func printSSHConfig(name string, target *guestTarget) {
	fmt.Printf("Host %s\n", name)
	fmt.Printf("  HostName %s\n", target.IP)
	fmt.Printf("  User %s\n", target.User)
	fmt.Printf("  IdentityFile %s\n", target.Key)
	fmt.Println("  IdentitiesOnly yes")
	fmt.Println("  StrictHostKeyChecking no")
	fmt.Println("  UserKnownHostsFile /dev/null")
}

func runSSH(args []string) error {
	fs, stateDir, user, command, timeout, dryRun, printConfig := sshFlagSet()
	targetArg, err := parsePositional(fs, args)
	if err != nil {
		return err
	}
	t, err := resolveVM(targetArg, *stateDir)
	if err != nil {
		return err
	}
	if t.State == nil {
		name := targetArg
		if t.Config != nil {
			name = t.Config.Name
		}
		return fmt.Errorf("no state for %q at %s", name, t.Layout.StateFile)
	}
	target, err := resolveGuestTarget(t.Layout.ConfigPath, t.Layout.StateDir, *user, *timeout)
	if err != nil {
		return err
	}
	if *printConfig {
		printSSHConfig(t.State.Name, target)
		return nil
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

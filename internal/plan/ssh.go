package plan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// SSHRunner executes commands in a guest over SSH.
type SSHRunner struct {
	Host   string
	User   string
	Key    string
	Port   int
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	DryRun bool
}

func (r *SSHRunner) IsDryRun() bool { return r.DryRun }

func (r *SSHRunner) stdout() io.Writer {
	if r.Stdout != nil {
		return r.Stdout
	}
	return os.Stdout
}

func (r *SSHRunner) stderr() io.Writer {
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

func shellQuoteArg(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func remoteCommand(cmd Command) string {
	var prefix []string
	if cmd.Sudo {
		prefix = append(prefix, "sudo", "-n")
	}

	parts := append([]string{cmd.Name}, cmd.Args...)
	if len(cmd.Env) > 0 {
		parts = append([]string{"env"}, append(append([]string{}, cmd.Env...), parts...)...)
	}
	parts = append(prefix, parts...)

	for i, part := range parts {
		parts[i] = shellQuoteArg(part)
	}
	return strings.Join(parts, " ")
}

func (r *SSHRunner) sshCommand(ctx context.Context, remote string, stdin io.Reader) *exec.Cmd {
	args := []string{
		"-i", r.Key,
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "LogLevel=ERROR",
	}
	if r.Port != 0 && r.Port != 22 {
		args = append(args, "-p", strconv.Itoa(r.Port))
	}
	args = append(args, r.User+"@"+r.Host, remote)
	c := exec.CommandContext(ctx, "ssh", args...)
	c.Stdin = stdin
	if c.Stdin == nil {
		c.Stdin = r.Stdin
	}
	return c
}

func (r *SSHRunner) printDryRun(remote string) {
	fmt.Fprintf(r.stdout(), "+ ssh %s@%s %s\n", r.User, r.Host, remote)
}

func (r *SSHRunner) Run(ctx context.Context, cmd Command) error {
	remote := remoteCommand(cmd)
	if r.DryRun {
		r.printDryRun(remote)
		return nil
	}
	c := r.sshCommand(ctx, remote, cmd.Stdin)
	c.Stdout = r.stdout()
	c.Stderr = r.stderr()
	if err := c.Run(); err != nil {
		return fmt.Errorf("ssh %s@%s: %w", r.User, r.Host, err)
	}
	return nil
}

func (r *SSHRunner) Output(ctx context.Context, cmd Command) (string, error) {
	remote := remoteCommand(cmd)
	if r.DryRun {
		r.printDryRun(remote)
		return "", nil
	}
	c := r.sshCommand(ctx, remote, cmd.Stdin)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return string(out), fmt.Errorf("ssh %s@%s: %w: %s", r.User, r.Host, err, msg)
		}
		return string(out), fmt.Errorf("ssh %s@%s: %w", r.User, r.Host, err)
	}
	return string(out), nil
}

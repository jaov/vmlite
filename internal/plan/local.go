package plan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// LocalRunner executes commands on the current host. DryRun prints the
// effective command without running it.
type LocalRunner struct {
	DryRun bool
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	// Geteuid is injectable for tests; it defaults to os.Geteuid.
	Geteuid func() int
}

func (r *LocalRunner) euid() int {
	if r.Geteuid != nil {
		return r.Geteuid()
	}
	return os.Geteuid()
}

func (r *LocalRunner) stdout() io.Writer {
	if r.Stdout != nil {
		return r.Stdout
	}
	return os.Stdout
}

func (r *LocalRunner) stderr() io.Writer {
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

func (r *LocalRunner) dryRunPrint(cmd Command) {
	fmt.Fprintf(r.stdout(), "+ %s\n", cmd.effectiveString(r.euid() == 0))
}

func (r *LocalRunner) command(ctx context.Context, cmd Command) (*exec.Cmd, error) {
	name, args, env := cmd.effective(r.euid() == 0)
	if name == "sudo" {
		if _, err := exec.LookPath("sudo"); err != nil {
			return nil, fmt.Errorf("sudo is required for %q but was not found: %w", cmd.Name, err)
		}
	}

	c := exec.CommandContext(ctx, name, args...)
	c.Env = append(os.Environ(), env...)
	if cmd.Stdin != nil {
		c.Stdin = cmd.Stdin
	} else {
		c.Stdin = r.Stdin
	}
	return c, nil
}

func (r *LocalRunner) Run(ctx context.Context, cmd Command) error {
	if r.DryRun {
		r.dryRunPrint(cmd)
		return nil
	}
	c, err := r.command(ctx, cmd)
	if err != nil {
		return err
	}
	c.Stdout = r.stdout()
	c.Stderr = r.stderr()
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s: %w", cmd.effectiveString(r.euid() == 0), err)
	}
	return nil
}

func (r *LocalRunner) Output(ctx context.Context, cmd Command) (string, error) {
	if r.DryRun {
		r.dryRunPrint(cmd)
		return "", nil
	}
	c, err := r.command(ctx, cmd)
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return string(out), fmt.Errorf("%s: %w: %s", cmd.effectiveString(r.euid() == 0), err, msg)
		}
		return string(out), fmt.Errorf("%s: %w", cmd.effectiveString(r.euid() == 0), err)
	}
	return string(out), nil
}

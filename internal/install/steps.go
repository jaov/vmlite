package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"vmlite/internal/plan"
)

// VerifyStep ensures the required binaries are available.
func VerifyStep(binaries []string) plan.Step {
	return plan.Ensure("verify required host tools", func(ctx context.Context, r plan.Runner) (bool, error) {
		for _, bin := range binaries {
			_, err := r.Output(ctx, plan.Command{
				Name: "sh",
				Args: []string{"-c", `command -v "$1"`, "sh", bin},
			})
			if err != nil {
				return false, nil
			}
		}
		return true, nil
	}, func(context.Context, plan.Runner) error {
		return fmt.Errorf("one or more required tools are missing: %s", strings.Join(binaries, ", "))
	})
}

// KVMStep reports whether /dev/kvm is available.
func KVMStep() plan.Step {
	return plan.Ensure("check /dev/kvm", func(context.Context, plan.Runner) (bool, error) {
		_, err := os.Stat("/dev/kvm")
		return err == nil, nil
	}, func(context.Context, plan.Runner) error {
		fmt.Fprintln(os.Stderr, "warning: /dev/kvm is missing; enable virtualization and load kvm_intel/kvm_amd")
		return nil
	})
}

func targetUser() string {
	if user := os.Getenv("SUDO_USER"); user != "" {
		return user
	}
	return os.Getenv("USER")
}

// GroupsStep adds the invoking user to libvirt/kvm when those groups exist.
func GroupsStep() plan.Step {
	return plan.Ensure("add user to libvirt/kvm groups", func(ctx context.Context, r plan.Runner) (bool, error) {
		user := targetUser()
		if user == "" || user == "root" {
			return true, nil
		}
		groupsOut, err := r.Output(ctx, plan.Command{Name: "id", Args: []string{"-nG", user}})
		if err != nil {
			return false, nil
		}
		groups := map[string]bool{}
		for _, group := range strings.Fields(groupsOut) {
			groups[group] = true
		}
		for _, group := range []string{"libvirt", "kvm"} {
			if _, err := r.Output(ctx, plan.Command{Name: "getent", Args: []string{"group", group}}); err != nil {
				continue
			}
			if !groups[group] {
				return false, nil
			}
		}
		return true, nil
	}, func(ctx context.Context, r plan.Runner) error {
		user := targetUser()
		if user == "" || user == "root" {
			return nil
		}
		var existing []string
		for _, group := range []string{"libvirt", "kvm"} {
			if _, err := r.Output(ctx, plan.Command{Name: "getent", Args: []string{"group", group}}); err == nil {
				existing = append(existing, group)
			}
		}
		if len(existing) == 0 {
			return nil
		}
		return r.Run(ctx, plan.Command{
			Name: "usermod",
			Args: []string{"-aG", strings.Join(existing, ","), user},
			Sudo: true,
		})
	})
}

// DefaultNetworkStep starts the default libvirt network when defined.
func DefaultNetworkStep() plan.Step {
	return plan.Ensure("ensure default libvirt network", func(ctx context.Context, r plan.Runner) (bool, error) {
		out, err := r.Output(ctx, plan.Command{Name: "virsh", Args: []string{"net-info", "default"}})
		if err != nil {
			return true, nil
		}
		return strings.Contains(out, "Active: yes"), nil
	}, func(ctx context.Context, r plan.Runner) error {
		if err := r.Run(ctx, plan.Command{Name: "virsh", Args: []string{"net-start", "default"}, Sudo: true}); err != nil {
			return nil
		}
		return r.Run(ctx, plan.Command{Name: "virsh", Args: []string{"net-autostart", "default"}, Sudo: true})
	})
}

// BinaryStep installs a local binary at Dest.
type BinaryStep struct {
	Source string
	Dest   string
	Mode   os.FileMode
}

func (s BinaryStep) Describe() string {
	return fmt.Sprintf("install vmlite binary to %s", s.Dest)
}

func (s BinaryStep) Check(ctx context.Context, r plan.Runner) (bool, error) {
	if s.Source == s.Dest {
		return true, nil
	}
	if _, err := os.Stat(s.Source); err != nil {
		return false, fmt.Errorf("source binary %q: %w", s.Source, err)
	}
	if _, err := os.Stat(s.Dest); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	srcHash := sha256File(s.Source)
	dstHash := sha256File(s.Dest)
	return srcHash != "" && srcHash == dstHash, nil
}

func (s BinaryStep) Apply(ctx context.Context, r plan.Runner) error {
	if dry, ok := r.(interface{ IsDryRun() bool }); ok && dry.IsDryRun() {
		fmt.Printf("+ install %s -> %s\n", s.Source, s.Dest)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Dest), 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}
	src, err := os.Open(s.Source)
	if err != nil {
		return fmt.Errorf("open source binary: %w", err)
	}
	defer src.Close()

	tmp, err := os.CreateTemp(filepath.Dir(s.Dest), ".vmlite-*")
	if err != nil {
		return fmt.Errorf("create install temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close install temp file: %w", err)
	}
	mode := s.Mode
	if mode == 0 {
		mode = 0o755
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod installed binary: %w", err)
	}
	if err := os.Rename(tmpName, s.Dest); err != nil {
		return fmt.Errorf("install binary to %q: %w", s.Dest, err)
	}
	return nil
}

func sha256File(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Package seed builds small read-only ISO volumes from named files. It is the
// primitive shared by cloud-init NoCloud seeds and unattended installer answer
// media.
package seed

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// File is a file placed at the root of a seed volume.
type File struct {
	Name string
	Data []byte
	Mode os.FileMode
}

// Volume describes a seed ISO: its filesystem label and root files.
type Volume struct {
	Label string
	Files []File
}

// Builder returns the ISO creation tool to use.
func Builder() (string, error) {
	for _, name := range []string{"xorrisofs", "genisoimage", "mkisofs"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("seed media requires xorrisofs, genisoimage, or mkisofs in PATH")
}

// WriteISO creates a Joliet/Rock Ridge ISO at path with the volume's label and
// files. The caller owns reuse and collision policy.
func WriteISO(volume Volume, path string) error {
	if strings.TrimSpace(volume.Label) == "" {
		return fmt.Errorf("seed volume label is required")
	}
	if len(volume.Files) == 0 {
		return fmt.Errorf("seed volume %q has no files", volume.Label)
	}
	for _, f := range volume.Files {
		if f.Name == "" || filepath.Base(f.Name) != f.Name {
			return fmt.Errorf("seed file %q must be a plain filename", f.Name)
		}
	}

	tool, err := Builder()
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp("", "vmlite-seed-")
	if err != nil {
		return fmt.Errorf("create seed workspace: %w", err)
	}
	defer os.RemoveAll(dir)

	for _, f := range volume.Files {
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(filepath.Join(dir, f.Name), f.Data, mode); err != nil {
			return fmt.Errorf("write seed file %q: %w", f.Name, err)
		}
	}

	cmd := exec.Command(tool,
		"-o", path,
		"-J",
		"-input-charset", "utf8",
		"-rational-rock",
		"-V", volume.Label,
		dir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create seed %q: %w: %s", path, err, strings.TrimSpace(string(out)))
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("chmod seed %q: %w", path, err)
	}
	return nil
}

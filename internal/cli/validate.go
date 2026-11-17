package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

func runValidate(args []string) error {
	fs := newStateFlagSet("validate")
	configPath, err := parseConfigArgs(fs, args)
	if err != nil {
		return err
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	if err := CheckImageTools(cfg); err != nil {
		return err
	}
	if err := validateLocalPaths(cfg); err != nil {
		return err
	}
	if _, err := cfg.InstallerSpec(); err != nil {
		return err
	}

	fmt.Printf("%s: valid\n", configPath)
	return nil
}

func validateLocalPaths(cfg *Config) error {
	switch cfg.Source.Type {
	case sourceCloudImage:
		if cfg.Source.Path != "" {
			if err := requirePath("source.path", cfg.Source.Path); err != nil {
				return err
			}
		}
	case sourceISO, sourceDisk:
		if cfg.Source.Path != "" {
			if err := requirePath("source.path", cfg.Source.Path); err != nil {
				return err
			}
		}
	}
	for i, d := range cfg.ExtraDisks {
		if err := requirePath(fmt.Sprintf("extra_disks[%d].path", i), d.Path); err != nil {
			return err
		}
	}
	for i, m := range cfg.Mounts {
		if err := requirePath(fmt.Sprintf("mounts[%d].source", i), m.Source); err != nil {
			return err
		}
	}
	if cfg.SSH.Key != "" {
		if err := requirePath("ssh.key", cfg.SSH.Key); err != nil {
			return err
		}
	}
	if cfg.Source.Installer != nil {
		if _, err := cfg.InstallerAnswerFile(); err != nil {
			return err
		}
	}
	return nil
}

func requirePath(field, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%s %q: %w", field, path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("%s %q is not accessible: %w", field, path, err)
	}
	return nil
}

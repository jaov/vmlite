package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config describes the virtual machine that vmlite creates.
type Config struct {
	Name       string  `json:"name"`
	Memory     int     `json:"memory"` // in MiB
	CPUs       int     `json:"cpus"`
	DiskSize   int     `json:"disk_size"` // in GiB
	OSVariant  string  `json:"os_variant"`
	InstallURL string  `json:"install_url,omitempty"`
	Mounts     []Mount `json:"mounts"`
}

// Mount is a host directory exposed to the guest.
type Mount struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// LoadConfig reads and validates a JSON configuration file.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %q: %w", path, err)
	}

	return &cfg, nil
}

// Validate checks that required values are present and sane.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if c.Memory <= 0 {
		return fmt.Errorf("memory must be greater than zero")
	}
	if c.CPUs <= 0 {
		return fmt.Errorf("cpus must be greater than zero")
	}
	if c.DiskSize <= 0 {
		return fmt.Errorf("disk_size must be greater than zero")
	}
	if strings.TrimSpace(c.OSVariant) == "" {
		return fmt.Errorf("os_variant is required")
	}
	for i, m := range c.Mounts {
		if strings.TrimSpace(m.Source) == "" {
			return fmt.Errorf("mounts[%d].source is required", i)
		}
		if strings.TrimSpace(m.Target) == "" {
			return fmt.Errorf("mounts[%d].target is required", i)
		}
	}
	return nil
}

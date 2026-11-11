package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	guestinstall "vmlite/internal/installer"
)

var errNotRunning = errors.New("VM is not running")

func runVirsh(uri string, args ...string) (string, error) {
	full := append([]string{"-c", uri}, args...)
	out, err := exec.Command("virsh", full...).CombinedOutput()
	return string(out), err
}

func domainStatus(uri, name string) (string, bool) {
	out, err := runVirsh(uri, "domstate", name)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
}

func domainUUID(uri, name string) string {
	out, err := runVirsh(uri, "domuuid", name)
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func resolveConnection(flagURI string) (string, error) {
	uri := flagURI
	if uri == "" {
		uri = os.Getenv("VMLITE_CONNECT")
	}
	if uri != "" {
		if err := ValidateConnection(uri); err != nil {
			return "", err
		}
		return uri, nil
	}
	return RunPreflightChecks()
}

func upFlagSet() (*flag.FlagSet, *string, *string, *bool, *bool, *bool) {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	connect := fs.String("connect", "", "libvirt connection URI")
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	recreate := fs.Bool("recreate", false, "destroy and recreate the VM")
	dryRun := fs.Bool("dry-run", false, "print actions without changing anything")
	offline := fs.Bool("offline", false, "do not download missing images")
	return fs, connect, stateDir, recreate, dryRun, offline
}

func runUp(args []string) error {
	fs, connect, stateDir, recreate, dryRun, offline := upFlagSet()
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return errUsage
	}
	configPath := "vm.json"
	if len(rest) == 1 {
		configPath = rest[0]
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	if err := CheckImageTools(cfg); err != nil {
		return err
	}

	layout, err := newProjectLayout(configPath, *stateDir, cfg.Name)
	if err != nil {
		return err
	}
	lock, err := acquireProjectLock(layout)
	if err != nil {
		return err
	}
	defer lock.Close()
	applyProjectDefaults(cfg, layout)

	configHash, err := hashConfig(cfg)
	if err != nil {
		return err
	}

	resolvedImage, err := resolveImage(cfg, *offline, *dryRun)
	if errors.Is(err, errImageWouldDownload) {
		fmt.Printf("Would download media %s into the cache\n", cfg.Source.URL)
		return nil
	}
	if err != nil {
		return err
	}
	if resolvedImage != "" {
		cfg.Source.Path = resolvedImage
	}

	uri, err := resolveConnection(*connect)
	if err != nil {
		return err
	}
	if len(cfg.Provisioners) > 0 && uri == sessionURI {
		return fmt.Errorf("shell provisioners require qemu:///system with a guest-visible network")
	}
	fmt.Printf("Using libvirt connection: %s\n", uri)

	st, err := loadState(layout.StateFile)
	if err != nil {
		return err
	}
	if st != nil {
		if st.ConfigPath != layout.ConfigPath {
			return fmt.Errorf("state %q belongs to config %q, not %q", layout.StateFile, st.ConfigPath, layout.ConfigPath)
		}
		if (st.ConfigHash != configHash || st.LibvirtURI != uri) && !*recreate {
			if *dryRun {
				fmt.Printf("Would recreate VM %q because its config or connection changed\n", cfg.Name)
				return nil
			}
			return fmt.Errorf("config or connection changed for %q; run destroy or use --recreate", cfg.Name)
		}
	}

	stateText, domainExists := domainStatus(uri, cfg.Name)

	if *recreate && st != nil {
		if *dryRun {
			fmt.Printf("Would destroy and recreate VM %q\n", cfg.Name)
			return nil
		}
		if err := destroyState(st, layout, true, false); err != nil {
			return err
		}
		st = nil
		domainExists = false
	}

	if st == nil {
		if domainExists {
			return fmt.Errorf("domain %q already exists but is not managed by vmlite (no state at %s)", cfg.Name, layout.StateFile)
		}
		if *dryRun {
			fmt.Printf("Would create VM %q via %s\n", cfg.Name, uri)
			return nil
		}
		if cfg.needsSSH() {
			if err := ensureSSHKey(cfg, layout); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(layout.AssetDir, 0o755); err != nil {
			return fmt.Errorf("create asset dir: %w", err)
		}
		if err := RunVirtInstall(cfg, uri); err != nil {
			return err
		}
		st, err = newState(cfg, layout, uri, configHash, domainUUID(uri, cfg.Name))
		if err != nil {
			return err
		}
		if err := saveState(layout.StateFile, st); err != nil {
			return fmt.Errorf("VM %q was created but state could not be written: %w", cfg.Name, err)
		}
		fmt.Printf("VM %q created successfully via %s\n", cfg.Name, uri)
		if cfg.hasInstaller() {
			fmt.Printf("Installer %q started; after it reboots, run: vmlite provision %s\n", cfg.Source.Installer.Type, configPath)
			if cfg.Source.Installer.Type == guestinstall.WindowsAutoUnattend {
				passwordPath := filepath.Join(layout.AssetDir, "windows-admin-password")
				if _, err := os.Stat(passwordPath); err == nil {
					fmt.Printf("Windows administrator password: %s\n", passwordPath)
				}
			}
			return nil
		}
		if len(cfg.Provisioners) > 0 {
			switch cfg.Source.Type {
			case sourceCloudImage, sourceDisk:
				if err := provisionVM(cfg, st, layout, false, false, 0); err != nil {
					return fmt.Errorf("VM %q was created but provisioning failed: %w; run vmlite provision after fixing it", cfg.Name, err)
				}
			case sourceISO:
				fmt.Printf("VM %q is installing manually; after the OS is installed and ssh.key is authorized, run: vmlite provision %s\n", cfg.Name, configPath)
			}
		}
		return nil
	}

	if len(cfg.Provisioners) > 0 && strings.TrimSpace(st.SSHKeyPath) == "" {
		return fmt.Errorf("VM %q predates provisioning support; use --recreate to enable shell provisioners", cfg.Name)
	}

	if !domainExists {
		if *dryRun {
			fmt.Printf("Would recreate VM %q because its domain is missing\n", cfg.Name)
			return nil
		}
		return fmt.Errorf("state exists for %q but its domain is missing; use --recreate", cfg.Name)
	}

	switch {
	case strings.Contains(stateText, "running"):
		if *dryRun {
			fmt.Printf("VM %q is already running\n", cfg.Name)
			return nil
		}
		fmt.Printf("VM %q is already running via %s\n", cfg.Name, uri)
		return nil
	case strings.Contains(stateText, "shut off"):
		if *dryRun {
			fmt.Printf("Would start VM %q\n", cfg.Name)
			return nil
		}
		if _, err := runVirsh(uri, "start", cfg.Name); err != nil {
			return fmt.Errorf("start VM %q: %w", cfg.Name, err)
		}
		st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := saveState(layout.StateFile, st); err != nil {
			return fmt.Errorf("VM %q started but state update failed: %w", cfg.Name, err)
		}
		fmt.Printf("VM %q started via %s\n", cfg.Name, uri)
		return nil
	default:
		return fmt.Errorf("VM %q is %q; expected running or shut off", cfg.Name, stateText)
	}
}

type statusJSON struct {
	Name                string            `json:"name"`
	ConfigPath          string            `json:"config_path,omitempty"`
	URI                 string            `json:"uri,omitempty"`
	State               string            `json:"state"`
	Running             bool              `json:"running"`
	Disk                string            `json:"disk,omitempty"`
	Image               string            `json:"image,omitempty"`
	IP                  string            `json:"ip,omitempty"`
	Installer           string            `json:"installer,omitempty"`
	InstallerCompletion string            `json:"installer_completion,omitempty"`
	InstallPhase        string            `json:"install_phase,omitempty"`
	Provisioned         []ProvisionRecord `json:"provisioned,omitempty"`
	UpdatedAt           string            `json:"updated_at,omitempty"`
}

func writeStatusJSON(w io.Writer, status statusJSON) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func statusFlagSet() (*flag.FlagSet, *string, *bool) {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	return fs, stateDir, jsonOut
}

func runStatus(args []string) error {
	fs, stateDir, jsonOut := statusFlagSet()
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return errUsage
	}
	configPath := "vm.json"
	if len(rest) == 1 {
		configPath = rest[0]
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	layout, err := newProjectLayout(configPath, *stateDir, cfg.Name)
	if err != nil {
		return err
	}
	st, err := loadState(layout.StateFile)
	if err != nil {
		return err
	}
	if st == nil {
		if *jsonOut {
			if err := writeStatusJSON(os.Stdout, statusJSON{
				Name:       cfg.Name,
				ConfigPath: layout.ConfigPath,
				State:      "not created",
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("VM %q has not been created (no state at %s)\n", cfg.Name, layout.StateFile)
		}
		return errNotRunning
	}

	stateText, exists := domainStatus(st.LibvirtURI, st.Name)
	if !exists {
		if *jsonOut {
			if err := writeStatusJSON(os.Stdout, statusJSON{
				Name:       st.Name,
				ConfigPath: st.ConfigPath,
				URI:        st.LibvirtURI,
				State:      "missing",
				Disk:       st.DiskPath,
				Image:      st.ImagePath,
				IP:         st.IP,
				Installer:  st.InstallerType,
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("VM %q: missing (state exists)\n", st.Name)
		}
		return errNotRunning
	}
	stateText = strings.TrimSpace(stateText)
	running := strings.Contains(stateText, "running")

	if *jsonOut {
		if err := writeStatusJSON(os.Stdout, statusJSON{
			Name:                st.Name,
			ConfigPath:          st.ConfigPath,
			URI:                 st.LibvirtURI,
			State:               stateText,
			Running:             running,
			Disk:                st.DiskPath,
			Image:               st.ImagePath,
			IP:                  st.IP,
			Installer:           st.InstallerType,
			InstallerCompletion: st.InstallerCompletion,
			InstallPhase:        st.InstallPhase,
			Provisioned:         st.Provisioned,
			UpdatedAt:           st.UpdatedAt,
		}); err != nil {
			return err
		}
		if !running {
			return errNotRunning
		}
		return nil
	}

	fmt.Printf("Name:  %s\n", st.Name)
	fmt.Printf("URI:   %s\n", st.LibvirtURI)
	fmt.Printf("State: %s\n", stateText)
	if st.DiskPath != "" {
		fmt.Printf("Disk:  %s\n", st.DiskPath)
	}
	if st.ImagePath != "" {
		fmt.Printf("Image: %s\n", st.ImagePath)
	}
	if st.IP != "" {
		fmt.Printf("IP:    %s\n", st.IP)
	}
	if st.InstallerType != "" {
		fmt.Printf("Installer: %s\n", st.InstallerType)
	}
	if st.InstallerType == guestinstall.WindowsAutoUnattend && st.AssetDir != "" {
		passwordPath := filepath.Join(st.AssetDir, "windows-admin-password")
		if _, err := os.Stat(passwordPath); err == nil {
			fmt.Printf("Windows password: %s\n", passwordPath)
		}
	}
	if st.InstallerISOPath != "" {
		fmt.Printf("Installer ISO: %s\n", st.InstallerISOPath)
	}
	if st.InstallPhase != "" {
		fmt.Printf("Install:   %s\n", st.InstallPhase)
	}
	if len(st.Provisioned) > 0 {
		fmt.Printf("Provisioned: %d script(s)\n", len(st.Provisioned))
	}
	if strings.Contains(stateText, "running") {
		return nil
	}
	return errNotRunning
}

func destroyFlagSet() (*flag.FlagSet, *string, *bool, *bool) {
	fs := flag.NewFlagSet("destroy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	force := fs.Bool("force", false, "destroy a running VM")
	dryRun := fs.Bool("dry-run", false, "print actions without changing anything")
	return fs, stateDir, force, dryRun
}

func runDestroy(args []string) error {
	fs, stateDir, force, dryRun := destroyFlagSet()
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return errUsage
	}
	configPath := "vm.json"
	if len(rest) == 1 {
		configPath = rest[0]
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	layout, err := newProjectLayout(configPath, *stateDir, cfg.Name)
	if err != nil {
		return err
	}
	lock, err := acquireProjectLock(layout)
	if err != nil {
		return err
	}
	defer lock.Close()
	st, err := loadState(layout.StateFile)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("no state for %q at %s", cfg.Name, layout.StateFile)
	}
	return destroyState(st, layout, *force, *dryRun)
}

func destroyState(st *State, layout projectLayout, force, dryRun bool) error {
	stateText, exists := domainStatus(st.LibvirtURI, st.Name)
	running := exists && strings.Contains(stateText, "running")
	if running && !force {
		return fmt.Errorf("VM %q is running; use --force to destroy it", st.Name)
	}
	if dryRun {
		fmt.Printf("Would destroy VM %q via %s and remove its managed files\n", st.Name, st.LibvirtURI)
		return nil
	}

	if running {
		if _, err := runVirsh(st.LibvirtURI, "destroy", st.Name); err != nil {
			return fmt.Errorf("destroy VM %q: %w", st.Name, err)
		}
	}
	if exists {
		if out, err := runVirsh(st.LibvirtURI, "undefine", st.Name, "--remove-all-storage", "--nvram"); err != nil {
			if out2, err2 := runVirsh(st.LibvirtURI, "undefine", st.Name, "--remove-all-storage"); err2 != nil {
				return fmt.Errorf("undefine VM %q: %w: %s", st.Name, err2, strings.TrimSpace(out+out2))
			}
		}
	}

	if st.DiskPath != "" {
		_ = os.Remove(st.DiskPath)
	}
	if st.SeedPath != "" {
		_ = os.Remove(st.SeedPath)
	}
	if st.InstallerISOPath != "" {
		_ = os.Remove(st.InstallerISOPath)
	}
	if st.AssetDir != "" && withinDir(filepath.Dir(layout.StateFile), st.AssetDir) {
		_ = os.RemoveAll(st.AssetDir)
	}
	if st.PoolName != "" {
		_, _ = runVirsh(st.LibvirtURI, "pool-destroy", st.PoolName)
		_, _ = runVirsh(st.LibvirtURI, "pool-undefine", st.PoolName)
	}
	if err := os.Remove(layout.StateFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove state %q: %w", layout.StateFile, err)
	}

	fmt.Printf("VM %q destroyed\n", st.Name)
	return nil
}

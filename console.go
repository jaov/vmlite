package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func consoleFlagSet() (*flag.FlagSet, *string, *string, *bool) {
	fs := newStateFlagSet("console")
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	consoleType := fs.String("type", "auto", "console type: auto, serial, or vnc")
	dryRun := fs.Bool("dry-run", false, "print actions without attaching")
	return fs, stateDir, consoleType, dryRun
}

func runConsole(args []string) error {
	fs, stateDir, consoleType, dryRun := consoleFlagSet()
	configPath, err := parseConfigArgs(fs, args)
	if err != nil {
		return err
	}
	switch *consoleType {
	case "auto", "serial", "vnc":
	default:
		return fmt.Errorf("console type must be %q, %q, or %q", "auto", "serial", "vnc")
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
		return fmt.Errorf("no state for %q at %s", cfg.Name, layout.StateFile)
	}
	text, exists := domainStatus(st.LibvirtURI, st.Name)
	if !exists {
		return fmt.Errorf("VM %q is missing", st.Name)
	}
	if !strings.Contains(text, "running") {
		return fmt.Errorf("VM %q is not running; run vmlite up or vmlite start", st.Name)
	}

	display := ""
	if out, err := runVirsh(st.LibvirtURI, "domdisplay", st.Name); err == nil {
		display = strings.TrimSpace(out)
	}
	if *consoleType == "vnc" || (*consoleType == "auto" && display != "") {
		if display == "" {
			return fmt.Errorf("VM %q has no graphical display", st.Name)
		}
		if *dryRun {
			fmt.Printf("Would print display %s for VM %q\n", display, st.Name)
			return nil
		}
		label := "Display"
		if strings.HasPrefix(display, "vnc://") {
			label = "VNC"
		} else if strings.HasPrefix(display, "spice://") {
			label = "SPICE"
		}
		fmt.Printf("%s: %s\n", label, display)
		fmt.Printf("Connect with: virt-viewer --connect %s %s\n", st.LibvirtURI, st.Name)
		return nil
	}

	args = []string{"-c", st.LibvirtURI, "console", st.Name, "--force"}
	if *dryRun {
		fmt.Printf("+ virsh %s\n", strings.Join(args, " "))
		return nil
	}
	cmd := exec.Command("virsh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("virsh console %q: %w", st.Name, err)
	}
	return nil
}

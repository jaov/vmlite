package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func consoleFlagSet() (*flag.FlagSet, *string, *string, *bool, *bool, *string) {
	fs := newStateFlagSet("console")
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	consoleType := fs.String("type", "auto", "console type: auto, serial, or vnc")
	dryRun := fs.Bool("dry-run", false, "print actions without attaching")
	logEnabled := fs.Bool("log", false, "record serial console output to .vmlite/<name>/console.log")
	logFile := fs.String("log-file", "", "serial console log path (implies --log)")
	return fs, stateDir, consoleType, dryRun, logEnabled, logFile
}

func runConsole(args []string) error {
	fs, stateDir, consoleType, dryRun, logEnabled, logFile := consoleFlagSet()
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
		if *logEnabled || *logFile != "" {
			return fmt.Errorf("console logging requires a serial console")
		}
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
	logPath := ""
	if *logEnabled || *logFile != "" {
		logPath = consoleLogPath(st, layout, *logFile)
		if logPath == "" {
			return fmt.Errorf("could not determine serial console log path")
		}
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return fmt.Errorf("create console log directory: %w", err)
		}
	}
	if *dryRun {
		if logPath != "" {
			fmt.Printf("+ script -q -c %q %s\n", consoleShellJoin(append([]string{"virsh"}, args...)), logPath)
		} else {
			fmt.Printf("+ virsh %s\n", strings.Join(args, " "))
		}
		return nil
	}

	if logPath != "" {
		scriptPath, err := exec.LookPath("script")
		if err != nil {
			return fmt.Errorf("console logging requires the script utility: %w", err)
		}
		fmt.Printf("Recording serial console to %s\n", logPath)
		cmd := exec.Command(scriptPath, "-q", "-c", consoleShellJoin(append([]string{"virsh"}, args...)), logPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("serial console %q: %w", st.Name, err)
		}
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

// consoleLogPath returns the serial console log path, using flagPath when set.
func consoleLogPath(st *State, layout projectLayout, flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if st != nil && st.AssetDir != "" {
		return filepath.Join(st.AssetDir, "console.log")
	}
	if layout.AssetDir != "" {
		return filepath.Join(layout.AssetDir, "console.log")
	}
	return ""
}

func consoleShellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = consoleShellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func consoleShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

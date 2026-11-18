package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// vmTarget is a config and its managed state, resolved from either a config
// path or a VM name.
type vmTarget struct {
	Config *Config
	Layout projectLayout
	State  *State
}

const stateDirFlagHelp = "state directory (default: VMLITE_STATE_DIR, state_dir in the config, or .vmlite next to the config)"

func resolveConfig(arg, stateDirFlag string) (*Config, projectLayout, error) {
	configPath := strings.TrimSpace(arg)
	if configPath == "" {
		configPath = "vm.json"
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, projectLayout{}, err
	}
	layout, err := newProjectLayout(configPath, resolveStateDir(stateDirFlag, cfg), cfg.Name)
	if err != nil {
		return nil, projectLayout{}, err
	}
	return cfg, layout, nil
}

// resolveVM resolves a config path or a managed VM name. A config target may
// have no state yet; callers decide whether that is an error.
func resolveVM(arg, stateDirFlag string) (vmTarget, error) {
	if strings.TrimSpace(arg) == "" {
		arg = "vm.json"
	}
	if isConfigArg(arg) {
		cfg, layout, err := resolveConfig(arg, stateDirFlag)
		if err != nil {
			return vmTarget{}, err
		}
		st, err := loadState(layout.StateFile)
		if err != nil {
			return vmTarget{}, err
		}
		return vmTarget{Config: cfg, Layout: layout, State: st}, nil
	}
	if !isBareVMName(arg) {
		return vmTarget{}, fmt.Errorf("invalid VM name %q", arg)
	}

	stateDir, err := stateDirForName(stateDirFlag)
	if err != nil {
		return vmTarget{}, err
	}
	stateFile := filepath.Join(stateDir, arg+".json")
	st, err := loadState(stateFile)
	if err != nil {
		return vmTarget{}, err
	}
	if st == nil {
		return vmTarget{}, fmt.Errorf("no VM named %q in %s", arg, stateDir)
	}
	if st.Name != arg {
		return vmTarget{}, fmt.Errorf("state %q belongs to VM %q, not %q", stateFile, st.Name, arg)
	}
	var cfg *Config
	if _, err := os.Stat(st.ConfigPath); err == nil {
		cfg, err = LoadConfig(st.ConfigPath)
		if err != nil {
			return vmTarget{}, err
		}
	} else if !os.IsNotExist(err) {
		return vmTarget{}, err
	}
	layout, err := newProjectLayout(st.ConfigPath, stateDir, arg)
	if err != nil {
		return vmTarget{}, err
	}
	return vmTarget{Config: cfg, Layout: layout, State: st}, nil
}

func resolveStateDir(flag string, cfg *Config) string {
	if strings.TrimSpace(flag) != "" {
		return flag
	}
	if env := strings.TrimSpace(os.Getenv("VMLITE_STATE_DIR")); env != "" {
		return env
	}
	if cfg == nil || strings.TrimSpace(cfg.StateDir) == "" {
		return ""
	}
	if filepath.IsAbs(cfg.StateDir) {
		return cfg.StateDir
	}
	return filepath.Join(cfg.ConfigDir, cfg.StateDir)
}

func stateDirForName(flag string) (string, error) {
	if dir := resolveStateDir(flag, nil); dir != "" {
		return filepath.Abs(dir)
	}
	if cfg, err := LoadConfig("vm.json"); err == nil {
		if dir := resolveStateDir("", cfg); dir != "" {
			return filepath.Abs(dir)
		}
		return filepath.Abs(filepath.Join(cfg.ConfigDir, ".vmlite"))
	}
	return filepath.Abs(".vmlite")
}

func isConfigArg(arg string) bool {
	if strings.HasSuffix(arg, ".json") {
		return true
	}
	_, err := os.Stat(arg)
	return err == nil || !os.IsNotExist(err)
}

func isBareVMName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ".json") {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

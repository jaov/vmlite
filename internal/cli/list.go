package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

type listEntry struct {
	Name         string `json:"name"`
	ConfigPath   string `json:"config_path,omitempty"`
	URI          string `json:"uri,omitempty"`
	State        string `json:"state"`
	Running      bool   `json:"running"`
	IP           string `json:"ip,omitempty"`
	Installer    string `json:"installer,omitempty"`
	InstallPhase string `json:"install_phase,omitempty"`
	Provisioned  int    `json:"provisioned"`
}

func listFlagSet() (*flag.FlagSet, *string, *bool) {
	fs := newStateFlagSet("list")
	stateDir := fs.String("state-dir", "", "state directory (default: VMLITE_STATE_DIR, state_dir in ./vm.json, or .vmlite in the current directory)")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	return fs, stateDir, jsonOut
}

func runList(args []string) error {
	fs, stateDir, jsonOut := listFlagSet()
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	if fs.NArg() > 0 {
		return errUsage
	}
	dir := resolveStateDir(*stateDir, nil)
	if dir == "" {
		if cfg, err := LoadConfig("vm.json"); err == nil {
			dir = resolveStateDir("", cfg)
		}
	}
	if dir == "" {
		dir = ".vmlite"
	}
	entries, err := listStates(dir)
	if err != nil {
		return err
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	if len(entries) == 0 {
		fmt.Println("no VMs")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tIP\tINSTALLER\tPHASE\tPROVISIONED")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n", e.Name, e.State, e.IP, e.Installer, e.InstallPhase, e.Provisioned)
	}
	return w.Flush()
}

func listStates(dir string) ([]listEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []listEntry{}, nil
		}
		return nil, fmt.Errorf("read state directory %q: %w", dir, err)
	}
	result := make([]listEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		st, err := loadState(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if st == nil {
			continue
		}
		stateText, exists := domainStatus(st.LibvirtURI, st.Name)
		state := "missing"
		running := false
		if exists {
			state = strings.TrimSpace(stateText)
			running = strings.Contains(state, "running")
		}
		result = append(result, listEntry{
			Name:         st.Name,
			ConfigPath:   st.ConfigPath,
			URI:          st.LibvirtURI,
			State:        state,
			Running:      running,
			IP:           st.IP,
			Installer:    st.InstallerType,
			InstallPhase: st.InstallPhase,
			Provisioned:  len(st.Provisioned),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

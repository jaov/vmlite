package cli

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"vmlite/internal/libvirt"
)

type inspectPaths struct {
	Config       string `json:"config,omitempty"`
	State        string `json:"state,omitempty"`
	Assets       string `json:"assets,omitempty"`
	Disk         string `json:"disk,omitempty"`
	Image        string `json:"image,omitempty"`
	Seed         string `json:"seed,omitempty"`
	InstallerISO string `json:"installer_iso,omitempty"`
	SSHKey       string `json:"ssh_key,omitempty"`
}

type inspectDisk struct {
	Target       string   `json:"target,omitempty"`
	Bus          string   `json:"bus,omitempty"`
	Device       string   `json:"device,omitempty"`
	Format       string   `json:"format,omitempty"`
	Source       string   `json:"source,omitempty"`
	ReadOnly     bool     `json:"readonly,omitempty"`
	Capacity     string   `json:"capacity,omitempty"`
	Allocation   string   `json:"allocation,omitempty"`
	Physical     string   `json:"physical,omitempty"`
	VirtualSize  int64    `json:"virtual_size,omitempty"`
	ActualSize   int64    `json:"actual_size,omitempty"`
	Backing      string   `json:"backing,omitempty"`
	BackingChain []string `json:"backing_chain,omitempty"`
}

type inspectInterface struct {
	Type    string `json:"type,omitempty"`
	MAC     string `json:"mac,omitempty"`
	Network string `json:"network,omitempty"`
	Bridge  string `json:"bridge,omitempty"`
	Model   string `json:"model,omitempty"`
	Target  string `json:"target,omitempty"`
}

type inspectInstaller struct {
	Type       string `json:"type,omitempty"`
	Completion string `json:"completion,omitempty"`
	Phase      string `json:"phase,omitempty"`
}

type inspectReport struct {
	Name        string             `json:"name"`
	ConfigPath  string             `json:"config_path,omitempty"`
	StatePath   string             `json:"state_path,omitempty"`
	URI         string             `json:"uri,omitempty"`
	UUID        string             `json:"uuid,omitempty"`
	State       string             `json:"state"`
	Running     bool               `json:"running"`
	MemoryMiB   int64              `json:"memory_mib,omitempty"`
	VCPUs       int                `json:"vcpus,omitempty"`
	Paths       inspectPaths       `json:"paths"`
	Disks       []inspectDisk      `json:"disks,omitempty"`
	Media       []inspectDisk      `json:"media,omitempty"`
	Network     []inspectInterface `json:"network,omitempty"`
	Addresses   []string           `json:"addresses,omitempty"`
	Installer   *inspectInstaller  `json:"installer,omitempty"`
	Provisioned []ProvisionRecord  `json:"provisioned,omitempty"`
	Stats       map[string]string  `json:"stats,omitempty"`
}

type qemuImgInfo struct {
	Filename              string `json:"filename"`
	Format                string `json:"format"`
	VirtualSize           int64  `json:"virtual-size"`
	ActualSize            int64  `json:"actual-size"`
	BackingFilename       string `json:"backing-filename"`
	FullBackingFilename   string `json:"full-backing-filename"`
	BackingFilenameFormat string `json:"backing-filename-format"`
}

func inspectFlagSet() (*flag.FlagSet, *string, *bool, *bool) {
	fs := newStateFlagSet("inspect")
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	xmlOut := fs.Bool("xml", false, "print the raw libvirt domain XML")
	return fs, stateDir, jsonOut, xmlOut
}

func runInspect(args []string) error {
	fs, stateDir, jsonOut, xmlOut := inspectFlagSet()
	configPath, err := parseConfigArgs(fs, args)
	if err != nil {
		return err
	}
	if *jsonOut && *xmlOut {
		return fmt.Errorf("--json and --xml are mutually exclusive")
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
		report := &inspectReport{
			Name:       cfg.Name,
			ConfigPath: layout.ConfigPath,
			StatePath:  layout.StateFile,
			State:      "not created",
			Paths: inspectPaths{
				Config: layout.ConfigPath,
				State:  layout.StateFile,
				Assets: layout.AssetDir,
			},
		}
		if *jsonOut {
			if err := writeInspectJSON(report); err != nil {
				return err
			}
		} else {
			fmt.Printf("VM %q has not been created (no state at %s)\n", cfg.Name, layout.StateFile)
		}
		return errNotRunning
	}

	if *xmlOut {
		if _, exists := domainStatus(st.LibvirtURI, st.Name); !exists {
			return fmt.Errorf("VM %q is missing", st.Name)
		}
		out, err := runVirsh(st.LibvirtURI, "dumpxml", st.Name)
		if err != nil {
			return fmt.Errorf("dumpxml %q: %w", st.Name, err)
		}
		fmt.Print(out)
		return nil
	}

	report := buildInspectReport(st, layout)
	if *jsonOut {
		if err := writeInspectJSON(report); err != nil {
			return err
		}
	} else {
		printInspectReport(report)
	}
	if !report.Running && report.State == "missing" {
		return errNotRunning
	}
	return nil
}

func buildInspectReport(st *State, layout projectLayout) *inspectReport {
	report := &inspectReport{
		Name:        st.Name,
		ConfigPath:  st.ConfigPath,
		StatePath:   layout.StateFile,
		URI:         st.LibvirtURI,
		UUID:        st.UUID,
		State:       "missing",
		Provisioned: st.Provisioned,
		Paths: inspectPaths{
			Config:       st.ConfigPath,
			State:        layout.StateFile,
			Assets:       layout.AssetDir,
			Disk:         st.DiskPath,
			Image:        st.ImagePath,
			Seed:         st.SeedPath,
			InstallerISO: st.InstallerISOPath,
			SSHKey:       st.SSHKeyPath,
		},
	}
	if st.InstallerType != "" {
		report.Installer = &inspectInstaller{
			Type:       st.InstallerType,
			Completion: st.InstallerCompletion,
			Phase:      st.InstallPhase,
		}
	}

	stateText, exists := domainStatus(st.LibvirtURI, st.Name)
	if !exists {
		if st.IP != "" {
			report.Addresses = []string{st.IP}
		}
		return report
	}
	report.State = strings.TrimSpace(stateText)
	report.Running = strings.Contains(report.State, "running")

	var domain libvirt.DomainXML
	if out, err := runVirsh(st.LibvirtURI, "dumpxml", st.Name); err == nil {
		if err := xml.Unmarshal([]byte(out), &domain); err == nil {
			report.UUID = domain.UUID
			if mib, err := libvirt.MemoryMiB(domain); err == nil {
				report.MemoryMiB = mib
			}
			report.VCPUs = domain.VCPU.Value
			for _, disk := range domain.Devices.Disks {
				item := inspectDOMDisk(st.LibvirtURI, st.Name, disk)
				if disk.Device == "cdrom" || disk.Device == "floppy" {
					report.Media = append(report.Media, item)
				} else {
					report.Disks = append(report.Disks, item)
				}
			}
			for _, iface := range domain.Devices.Interfaces {
				report.Network = append(report.Network, inspectInterface{
					Type:    iface.Type,
					MAC:     iface.MAC.Address,
					Network: iface.Source.Network,
					Bridge:  iface.Source.Bridge,
					Model:   iface.Model.Type,
					Target:  iface.Target.Dev,
				})
			}
		}
	}
	if report.UUID == "" {
		report.UUID = domainUUID(st.LibvirtURI, st.Name)
	}
	report.Addresses = domainAddresses(st.LibvirtURI, st.Name)
	if st.IP != "" && !containsString(report.Addresses, st.IP) {
		report.Addresses = append(report.Addresses, st.IP)
	}
	report.Stats = domainStats(st.LibvirtURI, st.Name)
	return report
}

func inspectDOMDisk(uri, name string, disk libvirt.DomainDisk) inspectDisk {
	item := inspectDisk{
		Target:   disk.Target.Dev,
		Bus:      disk.Target.Bus,
		Device:   disk.Device,
		Format:   disk.Driver.Type,
		Source:   disk.Source.File,
		ReadOnly: disk.ReadOnly != nil,
	}
	if item.Source == "" {
		item.Source = disk.Source.Dev
	}
	if disk.Target.Dev != "" {
		if out, err := runVirsh(uri, "domblkinfo", name, disk.Target.Dev); err == nil {
			capacity, allocation, physical := parseDomblkinfo(out)
			if capacity > 0 {
				item.Capacity = formatBytes(capacity)
			}
			if allocation > 0 {
				item.Allocation = formatBytes(allocation)
			}
			if physical > 0 {
				item.Physical = formatBytes(physical)
			}
		}
	}
	if item.Source != "" {
		if chain, err := qemuImgChain(item.Source); err == nil && len(chain) > 0 {
			item.Format = chain[0].Format
			item.VirtualSize = chain[0].VirtualSize
			item.ActualSize = chain[0].ActualSize
			item.Backing = chain[0].FullBackingFilename
			if item.Backing == "" {
				item.Backing = chain[0].BackingFilename
			}
			for _, entry := range chain {
				if entry.Filename != "" {
					item.BackingChain = append(item.BackingChain, entry.Filename)
				}
			}
		}
	}
	return item
}

func parseDomblkinfo(out string) (capacity, allocation, physical int64) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		var target *int64
		var value string
		switch {
		case strings.HasPrefix(line, "Capacity:"):
			target, value = &capacity, strings.TrimSpace(strings.TrimPrefix(line, "Capacity:"))
		case strings.HasPrefix(line, "Allocation:"):
			target, value = &allocation, strings.TrimSpace(strings.TrimPrefix(line, "Allocation:"))
		case strings.HasPrefix(line, "Physical:"):
			target, value = &physical, strings.TrimSpace(strings.TrimPrefix(line, "Physical:"))
		}
		if target != nil {
			if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
				*target = parsed
			}
		}
	}
	return capacity, allocation, physical
}

func formatBytes(value int64) string {
	switch {
	case value >= 1<<30:
		return fmt.Sprintf("%.3f GiB", float64(value)/(1<<30))
	case value >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(value)/(1<<20))
	case value >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(value)/(1<<10))
	default:
		return fmt.Sprintf("%d B", value)
	}
}

func qemuImgChain(path string) ([]qemuImgInfo, error) {
	out, err := exec.Command("qemu-img", "info", "--output=json", "--backing-chain", path).Output()
	if err != nil {
		return nil, err
	}
	var chain []qemuImgInfo
	if err := json.Unmarshal(out, &chain); err != nil {
		var single qemuImgInfo
		if err2 := json.Unmarshal(out, &single); err2 != nil {
			return nil, err
		}
		chain = []qemuImgInfo{single}
	}
	return chain, nil
}

func domainAddresses(uri, name string) []string {
	seen := map[string]bool{}
	var result []string
	for _, source := range []string{"lease", "arp"} {
		out, err := runVirsh(uri, "domifaddr", name, "--source", source)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if (field == "ipv4" || field == "ipv6") && i+1 < len(fields) {
					address := strings.Split(fields[i+1], "/")[0]
					if address != "" && !seen[address] {
						seen[address] = true
						result = append(result, address)
					}
				}
			}
		}
	}
	return result
}

func domainStats(uri, name string) map[string]string {
	out, err := runVirsh(uri, "domstats", name, "--state", "--cpu-total", "--block")
	if err != nil {
		return nil
	}
	stats := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		stats[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if len(stats) == 0 {
		return nil
	}
	return stats
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeInspectJSON(report *inspectReport) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func printInspectReport(report *inspectReport) {
	fmt.Printf("Name:   %s\n", report.Name)
	if report.ConfigPath != "" {
		fmt.Printf("Config: %s\n", report.ConfigPath)
	}
	if report.StatePath != "" {
		fmt.Printf("State:  %s\n", report.StatePath)
	}
	if report.URI != "" {
		fmt.Printf("URI:    %s\n", report.URI)
	}
	if report.UUID != "" {
		fmt.Printf("UUID:   %s\n", report.UUID)
	}
	fmt.Printf("Domain: %s\n", report.State)
	if report.VCPUs > 0 {
		fmt.Printf("CPU:    %d vCPU\n", report.VCPUs)
	}
	if report.MemoryMiB > 0 {
		fmt.Printf("Memory: %d MiB\n", report.MemoryMiB)
	}
	if len(report.Addresses) > 0 {
		fmt.Printf("IP:     %s\n", strings.Join(report.Addresses, ", "))
	}
	if value := report.Stats["cpu.time"]; value != "" {
		if nanos, err := strconv.ParseInt(value, 10, 64); err == nil {
			fmt.Printf("CPU time: %s\n", time.Duration(nanos).Round(time.Millisecond))
		}
	}

	printInspectPaths(report.Paths)
	if len(report.Disks) > 0 {
		fmt.Println("Disks:")
		for _, disk := range report.Disks {
			printInspectDisk(disk)
		}
	}
	if len(report.Media) > 0 {
		fmt.Println("Media:")
		for _, disk := range report.Media {
			printInspectDisk(disk)
		}
	}
	if len(report.Network) > 0 {
		fmt.Println("Network:")
		for _, iface := range report.Network {
			parts := []string{iface.Type}
			if iface.Network != "" {
				parts = append(parts, "network="+iface.Network)
			}
			if iface.Bridge != "" {
				parts = append(parts, "bridge="+iface.Bridge)
			}
			if iface.Model != "" {
				parts = append(parts, iface.Model)
			}
			if iface.MAC != "" {
				parts = append(parts, "mac="+iface.MAC)
			}
			fmt.Printf("  %s\n", strings.Join(parts, "  "))
		}
	}
	if report.Installer != nil {
		fmt.Println("Installer:")
		fmt.Printf("  %s (%s), phase %s\n", report.Installer.Type, report.Installer.Completion, report.Installer.Phase)
	}
	if len(report.Provisioned) > 0 {
		fmt.Println("Provisioned:")
		for _, record := range report.Provisioned {
			fmt.Printf("  %s  %s  %s\n", record.ID, record.SHA256, record.CompletedAt)
		}
	}
}

func printInspectPaths(paths inspectPaths) {
	entries := []struct {
		label string
		value string
	}{
		{"Assets", paths.Assets},
		{"Disk", paths.Disk},
		{"Image", paths.Image},
		{"Seed", paths.Seed},
		{"Installer ISO", paths.InstallerISO},
		{"SSH key", paths.SSHKey},
	}
	hasAny := false
	for _, entry := range entries {
		if entry.value != "" {
			if !hasAny {
				fmt.Println("Paths:")
				hasAny = true
			}
			fmt.Printf("  %-13s %s\n", entry.label+":", entry.value)
		}
	}
}

func printInspectDisk(disk inspectDisk) {
	parts := []string{}
	if disk.Target != "" {
		parts = append(parts, disk.Target)
	}
	if disk.Bus != "" {
		parts = append(parts, disk.Bus)
	}
	if disk.Format != "" {
		parts = append(parts, disk.Format)
	}
	if disk.Device != "" {
		parts = append(parts, disk.Device)
	}
	if disk.ReadOnly {
		parts = append(parts, "readonly")
	}
	if disk.Capacity != "" {
		size := disk.Capacity
		if disk.Allocation != "" {
			size += " capacity, " + disk.Allocation + " allocated"
		}
		parts = append(parts, size)
	}
	fmt.Printf("  %s\n", strings.Join(parts, "  "))
	if disk.Source != "" {
		fmt.Printf("    source: %s\n", disk.Source)
	}
	if disk.Backing != "" {
		fmt.Printf("    backing: %s\n", disk.Backing)
	}
}

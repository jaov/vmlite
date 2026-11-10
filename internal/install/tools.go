package install

// Tool describes a host capability and its package names per family.
type Tool struct {
	Name     string
	Binaries []string
	Packages map[string][]string
}

// Catalog is the host tool set vmlite needs.
var Catalog = []Tool{
	{
		Name:     "qemu",
		Binaries: []string{"qemu-img", "qemu-system-x86_64"},
		Packages: map[string][]string{
			"debian": {"qemu-system-x86", "qemu-utils"},
			"fedora": {"qemu-kvm", "qemu-img"},
			"arch":   {"qemu-full"},
			"suse":   {"qemu-kvm", "qemu-tools"},
		},
	},
	{
		Name:     "libvirt",
		Binaries: []string{"virsh"},
		Packages: map[string][]string{
			"debian": {"libvirt-daemon-system", "libvirt-clients"},
			"fedora": {"libvirt", "libvirt-daemon-kvm", "libvirt-daemon-config-network", "libvirt-client"},
			"arch":   {"libvirt", "dnsmasq"},
			"suse":   {"libvirt-daemon-qemu", "libvirt-client"},
		},
	},
	{
		Name:     "virt-install",
		Binaries: []string{"virt-install"},
		Packages: map[string][]string{
			"debian": {"virtinst"},
			"fedora": {"virt-install"},
			"arch":   {"virt-install"},
			"suse":   {"virt-install"},
		},
	},
	{
		Name:     "iso",
		Binaries: []string{"xorriso", "xorrisofs"},
		Packages: map[string][]string{
			"debian": {"xorriso"},
			"fedora": {"xorriso"},
			"arch":   {"xorriso"},
			"suse":   {"xorriso"},
		},
	},
	{
		Name:     "ssh",
		Binaries: []string{"ssh", "scp", "ssh-keygen"},
		Packages: map[string][]string{
			"debian": {"openssh-client"},
			"fedora": {"openssh-clients"},
			"arch":   {"openssh"},
			"suse":   {"openssh-clients"},
		},
	},
}

// PackagesFor returns the deduplicated package list for a family.
func PackagesFor(family string) []string {
	var result []string
	seen := map[string]bool{}
	for _, tool := range Catalog {
		for _, pkg := range tool.Packages[family] {
			if !seen[pkg] {
				seen[pkg] = true
				result = append(result, pkg)
			}
		}
	}
	return result
}

// BinariesFor returns the deduplicated binary list.
func BinariesFor() []string {
	var result []string
	seen := map[string]bool{}
	for _, tool := range Catalog {
		for _, bin := range tool.Binaries {
			if !seen[bin] {
				seen[bin] = true
				result = append(result, bin)
			}
		}
	}
	return result
}

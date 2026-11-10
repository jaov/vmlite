package install

import "testing"

func TestPackagesForDebian(t *testing.T) {
	pkgs := PackagesFor("debian")
	want := []string{"qemu-system-x86", "libvirt-daemon-system", "virtinst", "xorriso", "openssh-client"}
	for _, pkg := range want {
		found := false
		for _, got := range pkgs {
			if got == pkg {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("debian packages missing %q: %v", pkg, pkgs)
		}
	}

	seen := map[string]bool{}
	for _, pkg := range pkgs {
		if seen[pkg] {
			t.Errorf("duplicate package %q", pkg)
		}
		seen[pkg] = true
	}
}

func TestBinariesFor(t *testing.T) {
	bins := BinariesFor()
	for _, want := range []string{"qemu-img", "qemu-system-x86_64", "virsh", "virt-install", "xorrisofs", "ssh", "scp", "ssh-keygen"} {
		found := false
		for _, got := range bins {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("binaries missing %q: %v", want, bins)
		}
	}
}

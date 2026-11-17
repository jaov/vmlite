package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// installedVmlitePath locates the managed CLI. With a prefix it checks that
// directory; otherwise it follows PATH so package installs in /usr/bin work.
func installedVmlitePath(prefix string) (string, error) {
	if prefix != "" {
		path := filepath.Join(prefix, "vmlite")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("not installed at %s", path)
		}
		return path, nil
	}
	if path, err := exec.LookPath("vmlite"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("not found in PATH; use --prefix DIR")
}

// Doctor reports host readiness without changing anything.
func Doctor(p Platform, prefix string) error {
	fmt.Printf("Platform: %s (%s, %s)\n", p.ID, p.Family, p.Arch)

	var problems []string
	if _, err := os.Stat("/dev/kvm"); err != nil {
		problems = append(problems, "/dev/kvm is missing")
		fmt.Println("/dev/kvm: missing")
	} else {
		fmt.Println("/dev/kvm: present")
	}

	for _, bin := range BinariesFor() {
		path, err := exec.LookPath(bin)
		if err != nil {
			problems = append(problems, bin+" is missing")
			fmt.Printf("%s: missing\n", bin)
			continue
		}
		fmt.Printf("%s: %s\n", bin, path)
	}

	systemOK := exec.Command("virsh", "-c", "qemu:///system", "nodeinfo").Run() == nil
	sessionOK := exec.Command("virsh", "-c", "qemu:///session", "nodeinfo").Run() == nil
	switch {
	case systemOK:
		fmt.Println("libvirt: qemu:///system reachable")
	case sessionOK:
		fmt.Println("libvirt: qemu:///session reachable (system unavailable)")
	default:
		problems = append(problems, "neither qemu:///system nor qemu:///session is reachable")
		fmt.Println("libvirt: unavailable")
	}

	vmlite, err := installedVmlitePath(prefix)
	if err != nil {
		fmt.Printf("vmlite: %v\n", err)
	} else {
		fmt.Printf("vmlite: %s\n", vmlite)
	}

	if len(problems) > 0 {
		return fmt.Errorf("doctor found %d problem(s): %s", len(problems), strings.Join(problems, "; "))
	}
	fmt.Println("All checks passed")
	return nil
}

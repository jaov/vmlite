// Package libvirt contains the small libvirt/virsh surface vmlite needs.
package libvirt

import (
	"fmt"
	"os/exec"
	"strings"
)

// Run executes virsh against uri and returns combined output.
func Run(uri string, args ...string) (string, error) {
	full := append([]string{"-c", uri}, args...)
	out, err := exec.Command("virsh", full...).CombinedOutput()
	return string(out), err
}

// Status returns the domain state and whether the domain exists.
func Status(uri, name string) (string, bool) {
	out, err := Run(uri, "domstate", name)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// UUID returns the domain UUID, or an empty string when unavailable.
func UUID(uri, name string) string {
	out, err := Run(uri, "domuuid", name)
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// DomainXML is the subset of a libvirt domain definition used by vmlite and
// its integration tests.
type DomainXML struct {
	UUID   string `xml:"uuid"`
	Memory struct {
		Unit  string `xml:"unit,attr"`
		Value int64  `xml:",chardata"`
	} `xml:"memory"`
	VCPU struct {
		Value int `xml:",chardata"`
	} `xml:"vcpu"`
	Devices struct {
		Disks       []DomainDisk       `xml:"disk"`
		Interfaces  []DomainInterface  `xml:"interface"`
		Filesystems []DomainFilesystem `xml:"filesystem"`
	} `xml:"devices"`
}

// DomainDisk describes a libvirt disk device.
type DomainDisk struct {
	Type   string `xml:"type,attr"`
	Device string `xml:"device,attr"`
	Driver struct {
		Name string `xml:"name,attr"`
		Type string `xml:"type,attr"`
	} `xml:"driver"`
	Source struct {
		File string `xml:"file,attr"`
		Dev  string `xml:"dev,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
		Bus string `xml:"bus,attr"`
	} `xml:"target"`
	ReadOnly *string `xml:"readonly"`
}

// DomainInterface describes a libvirt network interface.
type DomainInterface struct {
	Type string `xml:"type,attr"`
	MAC  struct {
		Address string `xml:"address,attr"`
	} `xml:"mac"`
	Source struct {
		Network string `xml:"network,attr"`
		Bridge  string `xml:"bridge,attr"`
	} `xml:"source"`
	Model struct {
		Type string `xml:"type,attr"`
	} `xml:"model"`
	Target struct {
		Dev string `xml:"dev,attr"`
	} `xml:"target"`
}

// DomainFilesystem describes a libvirt filesystem/9p mount.
type DomainFilesystem struct {
	Type       string `xml:"type,attr"`
	AccessMode string `xml:"accessmode,attr"`
	Source     struct {
		Dir string `xml:"dir,attr"`
	} `xml:"source"`
	Target struct {
		Dir string `xml:"dir,attr"`
	} `xml:"target"`
}

// MemoryMiB converts the libvirt memory element to MiB.
func MemoryMiB(domain DomainXML) (int64, error) {
	switch strings.ToLower(strings.TrimSpace(domain.Memory.Unit)) {
	case "", "kib":
		return domain.Memory.Value / 1024, nil
	case "mib":
		return domain.Memory.Value, nil
	case "gib":
		return domain.Memory.Value * 1024, nil
	case "tib":
		return domain.Memory.Value * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("unexpected memory unit %q", domain.Memory.Unit)
	}
}

package main

import (
	"fmt"
	"strings"
)

type domainDisk struct {
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

type domainInterface struct {
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

type domainFilesystem struct {
	Type       string `xml:"type,attr"`
	AccessMode string `xml:"accessmode,attr"`
	Source     struct {
		Dir string `xml:"dir,attr"`
	} `xml:"source"`
	Target struct {
		Dir string `xml:"dir,attr"`
	} `xml:"target"`
}

type domainXML struct {
	UUID   string `xml:"uuid"`
	Memory struct {
		Unit  string `xml:"unit,attr"`
		Value int64  `xml:",chardata"`
	} `xml:"memory"`
	VCPU struct {
		Value int `xml:",chardata"`
	} `xml:"vcpu"`
	Devices struct {
		Disks       []domainDisk       `xml:"disk"`
		Interfaces  []domainInterface  `xml:"interface"`
		Filesystems []domainFilesystem `xml:"filesystem"`
	} `xml:"devices"`
}

func domainMemoryMiB(domain domainXML) (int64, error) {
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

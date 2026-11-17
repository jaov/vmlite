package install

import (
	"encoding/xml"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type poolInfo struct {
	Name   string
	Path   string
	Vmlite bool
}

// inspectPools lists libvirt pools and marks directories inside a vmlite
// state tree. It is read-only and never changes pool state.
func inspectPools(uri string) ([]poolInfo, error) {
	out, err := exec.Command("virsh", "-c", uri, "pool-list", "--all", "--name").Output()
	if err != nil {
		return nil, fmt.Errorf("list libvirt pools: %w", err)
	}
	var pools []poolInfo
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		info := poolInfo{Name: name}
		if dump, err := exec.Command("virsh", "-c", uri, "pool-dumpxml", name).Output(); err == nil {
			var pool struct {
				Target struct {
					Path string `xml:"path"`
				} `xml:"target"`
			}
			if xml.Unmarshal(dump, &pool) == nil {
				info.Path = pool.Target.Path
			}
		}
		info.Vmlite = strings.Contains(filepath.ToSlash(info.Path), "/.vmlite/")
		pools = append(pools, info)
	}
	sort.Slice(pools, func(i, j int) bool { return pools[i].Name < pools[j].Name })
	return pools, nil
}

func printPools(uri string) {
	pools, err := inspectPools(uri)
	if err != nil || len(pools) == 0 {
		return
	}
	fmt.Println("libvirt pools:")
	for _, pool := range pools {
		marker := ""
		if pool.Vmlite {
			marker = " [vmlite]"
		}
		if pool.Path != "" {
			fmt.Printf("%s  %s%s\n", pool.Name, pool.Path, marker)
		} else {
			fmt.Printf("%s%s\n", pool.Name, marker)
		}
	}
}

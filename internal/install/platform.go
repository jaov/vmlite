package install

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Platform describes the supported host distribution family.
type Platform struct {
	ID     string
	Like   string
	Family string
	Arch   string
	Root   bool
}

// DetectPlatform reads /etc/os-release or VMLITE_OS_RELEASE (test override).
func DetectPlatform() (Platform, error) {
	path := os.Getenv("VMLITE_OS_RELEASE")
	if path == "" {
		path = "/etc/os-release"
	}
	return DetectPlatformFrom(path)
}

// DetectPlatformFrom parses an os-release file.
func DetectPlatformFrom(path string) (Platform, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Platform{}, fmt.Errorf("read %s: %w", path, err)
	}

	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}

	id := values["ID"]
	like := values["ID_LIKE"]
	family, err := familyFrom(id, like)
	if err != nil {
		return Platform{}, err
	}

	return Platform{
		ID:     id,
		Like:   like,
		Family: family,
		Arch:   runtime.GOARCH,
		Root:   os.Geteuid() == 0,
	}, nil
}

func familyFrom(id, like string) (string, error) {
	tokens := strings.Fields(strings.ToLower(id + " " + like))
	has := func(name string) bool {
		for _, token := range tokens {
			if token == name {
				return true
			}
		}
		return false
	}

	switch {
	case has("freebsd"):
		return "", fmt.Errorf("FreeBSD is not supported as a vmlite host: Linux KVM, libvirt, and virt-install are required")
	case has("debian") || has("ubuntu"):
		return "debian", nil
	case has("fedora") || has("rhel") || has("centos") || has("rocky") || has("alma"):
		return "fedora", nil
	case has("arch"):
		return "arch", nil
	case has("opensuse") || has("suse") || has("sles"):
		return "suse", nil
	default:
		return "", fmt.Errorf("unsupported distribution: ID=%q ID_LIKE=%q", id, like)
	}
}

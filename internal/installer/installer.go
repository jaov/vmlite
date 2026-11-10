// Package installer describes unattended OS installers as a small spec of
// seed media and boot behavior. Distro-specific answer files live behind the
// Installer interface; callers only consume Spec.
package installer

import (
	"fmt"
	"strings"

	"vmlite/internal/seed"
)

const (
	// UbuntuAutoinstall is the Ubuntu live-server autoinstall installer.
	UbuntuAutoinstall = "ubuntu_autoinstall"
)

// Input is the host and account information an installer may inject.
type Input struct {
	Hostname   string
	SSHUser    string
	SSHKey     string
	AnswerFile []byte
}

// Spec is the complete, distro-independent recipe for unattended installation.
type Spec struct {
	Type       string
	Seed       *seed.Volume
	SeedDevice string   // disk or cdrom
	BootOrder  []string // final boot order, e.g. hd,cdrom
	KernelArgs []string
}

// Installer turns host/account input into a Spec.
type Installer interface {
	Type() string
	Spec(Input) (Spec, error)
}

// Lookup returns the installer registered for type.
func Lookup(typ string) (Installer, error) {
	switch typ {
	case UbuntuAutoinstall:
		return ubuntuAutoinstall{}, nil
	default:
		return nil, fmt.Errorf("unsupported installer.type %q", typ)
	}
}

func validateHostname(s string) error {
	if s == "" {
		return fmt.Errorf("installer hostname is required")
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '.' || r == '_' {
			continue
		}
		return fmt.Errorf("installer hostname %q contains invalid character %q", s, r)
	}
	return nil
}

func validateUser(s string) error {
	if s == "" {
		return fmt.Errorf("installer SSH user is required")
	}
	for i, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if i == 0 && r >= '0' && r <= '9' {
			ok = false
		}
		if !ok {
			return fmt.Errorf("installer SSH user %q contains invalid character %q", s, r)
		}
	}
	return nil
}

func replacePlaceholders(text string, in Input) string {
	return strings.NewReplacer(
		"{{VMLITE_HOSTNAME}}", in.Hostname,
		"{{VMLITE_SSH_USER}}", in.SSHUser,
		"{{VMLITE_SSH_AUTHORIZED_KEY}}", strings.TrimSpace(in.SSHKey),
	).Replace(text)
}

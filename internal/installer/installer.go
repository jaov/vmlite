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
	// FedoraKickstart is the Fedora Server kickstart installer.
	FedoraKickstart = "fedora_kickstart"
	// DebianPreseed is the Debian installer preseed provider.
	DebianPreseed = "debian_preseed"
	// WindowsAutoUnattend is the Windows Setup autounattend provider.
	WindowsAutoUnattend = "windows_autounattend"
)

// Input is the host and account information an installer may inject.
type Input struct {
	Hostname   string
	SSHUser    string
	SSHKey     string
	AnswerFile []byte
	Windows    *WindowsOptions
}

// Completion describes how vmlite knows an unattended install is finished.
type Completion string

const (
	// CompletionShutdown waits for the installer domain to shut off.
	CompletionShutdown Completion = "shutdown"
	// CompletionSSH waits for the installed guest to expose SSH.
	CompletionSSH Completion = "ssh"
)

// Spec is the complete, distro-independent recipe for unattended installation.
type Spec struct {
	Type       string
	Seed       *seed.Volume
	SeedDevice string   // disk or cdrom
	BootOrder  []string // final boot order, e.g. hd,cdrom
	KernelArgs []string
	// DiskBus is the default bus for installer primary disks. Empty means
	// virtio, preserving Linux installer behavior.
	DiskBus string
	// Completion selects the install-complete signal. Empty means shutdown.
	Completion Completion
	// InjectFiles are added to a remastered copy of the installer ISO. This is
	// used when an installer can only read its answer file from the install
	// media itself.
	InjectFiles []seed.File
}

// Installer turns host/account input into a Spec.
type Installer interface {
	Type() string
	Spec(Input) (Spec, error)
}

// AnswerFileRequirement marks installers that cannot generate a default answer
// file and require installer.config or installer.inline.
type AnswerFileRequirement interface {
	RequiresAnswerFile() bool
}

// Lookup returns the installer registered for type.
func Lookup(typ string) (Installer, error) {
	switch typ {
	case UbuntuAutoinstall:
		return ubuntuAutoinstall{}, nil
	case FedoraKickstart:
		return fedoraKickstart{}, nil
	case DebianPreseed:
		return debianPreseed{}, nil
	case WindowsAutoUnattend:
		return windowsAutoUnattend{}, nil
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

package installer

import (
	"fmt"

	"vmlite/internal/seed"
)

type windowsAutoUnattend struct{}

func (windowsAutoUnattend) Type() string { return WindowsAutoUnattend }

// DefaultSSHUser returns the Windows administrative account used by OpenSSH.
func (windowsAutoUnattend) DefaultSSHUser() string { return "Administrator" }

// RequiresAnswerFile reports that Windows Setup needs a supplied autounattend.xml.
func (windowsAutoUnattend) RequiresAnswerFile() bool { return true }

func (windowsAutoUnattend) Spec(in Input) (Spec, error) {
	if err := validateHostname(in.Hostname); err != nil {
		return Spec{}, err
	}
	if err := validateUser(in.SSHUser); err != nil {
		return Spec{}, err
	}
	if len(in.AnswerFile) == 0 {
		return Spec{}, fmt.Errorf("windows_autounattend requires installer.config or installer.inline with an autounattend.xml")
	}

	answer := []byte(replacePlaceholders(string(in.AnswerFile), in))
	return Spec{
		Type: WindowsAutoUnattend,
		Seed: &seed.Volume{
			Label: "VMLITE",
			Files: []seed.File{{Name: "autounattend.xml", Data: answer}},
		},
		SeedDevice: "cdrom",
		BootOrder:  []string{"hd", "cdrom"},
		DiskBus:    "sata",
		Completion: CompletionSSH,
	}, nil
}

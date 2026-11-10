package installer

import (
	"vmlite/internal/seed"
)

type windowsAutoUnattend struct{}

func (windowsAutoUnattend) Type() string { return WindowsAutoUnattend }

// DefaultSSHUser returns the Windows administrative account used by OpenSSH.
func (windowsAutoUnattend) DefaultSSHUser() string { return "Administrator" }

// RequiresAnswerFile reports that Windows Setup needs a supplied autounattend.xml.
func (windowsAutoUnattend) RequiresAnswerFile() bool { return false }

func (windowsAutoUnattend) Spec(in Input) (Spec, error) {
	if err := validateHostname(in.Hostname); err != nil {
		return Spec{}, err
	}
	if err := validateUser(in.SSHUser); err != nil {
		return Spec{}, err
	}

	answer := in.AnswerFile
	if len(answer) == 0 {
		var err error
		answer, err = renderWindowsAutoUnattend(in)
		if err != nil {
			return Spec{}, err
		}
	} else {
		answer = []byte(replacePlaceholders(string(answer), in))
	}

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

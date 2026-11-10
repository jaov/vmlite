package installer

import (
	"fmt"
	"strings"

	"vmlite/internal/seed"
)

type ubuntuAutoinstall struct{}

func (ubuntuAutoinstall) Type() string { return UbuntuAutoinstall }

func (ubuntuAutoinstall) Spec(in Input) (Spec, error) {
	if err := validateHostname(in.Hostname); err != nil {
		return Spec{}, err
	}
	if err := validateUser(in.SSHUser); err != nil {
		return Spec{}, err
	}

	userData := in.AnswerFile
	if len(userData) == 0 {
		var err error
		userData, err = defaultUbuntuAutoinstall(in)
		if err != nil {
			return Spec{}, err
		}
	} else {
		userData = []byte(replacePlaceholders(string(userData), in))
	}

	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", in.Hostname, in.Hostname)
	return Spec{
		Type: UbuntuAutoinstall,
		Seed: &seed.Volume{
			Label: "cidata",
			Files: []seed.File{
				{Name: "user-data", Data: userData},
				{Name: "meta-data", Data: []byte(metaData)},
			},
		},
		SeedDevice: "disk",
		BootOrder:  []string{"hd", "cdrom"},
		KernelArgs: []string{"autoinstall"},
	}, nil
}

func defaultUbuntuAutoinstall(in Input) ([]byte, error) {
	var sshBlock string
	if strings.TrimSpace(in.SSHKey) != "" {
		sshBlock = fmt.Sprintf(`  ssh:
    install-server: true
    allow-pw: false
    authorized-keys:
      - %s
`, strings.TrimSpace(in.SSHKey))
	} else {
		sshBlock = `  ssh:
    install-server: false
    allow-pw: false
`
	}

	var lateCommands string
	if strings.TrimSpace(in.SSHKey) != "" {
		lateCommands = fmt.Sprintf(`  late-commands:
    - curtin in-target -- sh -c 'echo "%s ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/90-vmlite'
    - curtin in-target -- chmod 440 /etc/sudoers.d/90-vmlite
`, in.SSHUser)
	}

	text := fmt.Sprintf(`#cloud-config
autoinstall:
  version: 1
  identity:
    hostname: %s
    username: %s
    password: "*"
%s  storage:
    layout:
      name: direct
%s`, in.Hostname, in.SSHUser, sshBlock, lateCommands)
	return []byte(text), nil
}

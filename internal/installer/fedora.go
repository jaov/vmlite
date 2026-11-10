package installer

import (
	"fmt"
	"strings"

	"vmlite/internal/seed"
)

type fedoraKickstart struct{}

func (fedoraKickstart) Type() string { return FedoraKickstart }

func (fedoraKickstart) Spec(in Input) (Spec, error) {
	if err := validateHostname(in.Hostname); err != nil {
		return Spec{}, err
	}
	if err := validateUser(in.SSHUser); err != nil {
		return Spec{}, err
	}

	kickstart := in.AnswerFile
	if len(kickstart) == 0 {
		kickstart = []byte(defaultFedoraKickstart(in))
	} else {
		kickstart = []byte(replacePlaceholders(string(kickstart), in))
	}

	return Spec{
		Type: FedoraKickstart,
		Seed: &seed.Volume{
			Label: "OEMDRV",
			Files: []seed.File{{Name: "ks.cfg", Data: kickstart}},
		},
		SeedDevice: "disk",
		BootOrder:  []string{"hd", "cdrom"},
	}, nil
}

func defaultFedoraKickstart(in Input) string {
	var sshBits string
	if strings.TrimSpace(in.SSHKey) != "" {
		sshBits = fmt.Sprintf(`sshkey --username=%s "%s"
services --enabled=sshd
`, in.SSHUser, strings.TrimSpace(in.SSHKey))
	}

	return fmt.Sprintf(`text
cdrom
lang en_US.UTF-8
keyboard us
timezone UTC --utc
rootpw --lock
network --bootproto=dhcp --device=link --activate --hostname=%s
firewall --enabled --service=ssh
zerombr
clearpart --all --initlabel
autopart --type=plain
user --name=%s --groups=wheel --lock
%s%%packages
@^server-product-environment
%%end

%%post --erroronfail
echo '%%wheel ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-vmlite
chmod 440 /etc/sudoers.d/90-vmlite
%%end

reboot
`, in.Hostname, in.SSHUser, sshBits)
}

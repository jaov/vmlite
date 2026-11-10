package installer

import (
	"fmt"
	"strings"

	"vmlite/internal/seed"
)

type debianPreseed struct{}

func (debianPreseed) Type() string { return DebianPreseed }

func (debianPreseed) Spec(in Input) (Spec, error) {
	if err := validateHostname(in.Hostname); err != nil {
		return Spec{}, err
	}
	if err := validateUser(in.SSHUser); err != nil {
		return Spec{}, err
	}

	preseed := in.AnswerFile
	if len(preseed) == 0 {
		preseed = []byte(defaultDebianPreseed(in))
	} else {
		preseed = []byte(replacePlaceholders(string(preseed), in))
	}

	return Spec{
		Type: DebianPreseed,
		InjectFiles: []seed.File{
			{Name: "preseed.cfg", Data: preseed},
		},
		BootOrder: []string{"hd", "cdrom"},
		KernelArgs: []string{
			"auto=true",
			"priority=critical",
			"preseed/file=/cdrom/preseed.cfg",
		},
	}, nil
}

func defaultDebianPreseed(in Input) string {
	key := strings.TrimSpace(in.SSHKey)
	late := fmt.Sprintf(`d-i preseed/late_command string \
 in-target mkdir -p /home/%s/.ssh; \
 in-target sh -c 'echo "%s" > /home/%s/.ssh/authorized_keys'; \
 in-target chmod 700 /home/%s/.ssh; \
 in-target chmod 600 /home/%s/.ssh/authorized_keys; \
 in-target chown -R %s:%s /home/%s/.ssh; \
 in-target sh -c 'echo "%s ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/90-vmlite'; \
 in-target chmod 440 /etc/sudoers.d/90-vmlite; \
 in-target mkdir -p /etc/ssh/sshd_config.d; \
 in-target sh -c 'echo "PasswordAuthentication no" > /etc/ssh/sshd_config.d/99-vmlite.conf'; \
 in-target systemctl enable ssh
`, in.SSHUser, key, in.SSHUser, in.SSHUser, in.SSHUser,
		in.SSHUser, in.SSHUser, in.SSHUser, in.SSHUser)

	return fmt.Sprintf(`d-i debian-installer/locale string en_US.UTF-8
d-i keyboard-configuration/xkb-keymap select us
d-i netcfg/choose_interface select auto
d-i netcfg/get_hostname string %s
d-i netcfg/get_domain string localdomain
d-i mirror/country string manual
d-i mirror/http/hostname string deb.debian.org
d-i mirror/http/directory string /debian
d-i mirror/http/proxy string
d-i apt-setup/use_mirror boolean true
d-i apt-setup/cdrom/set-first boolean false
d-i apt-setup/cdrom/set-next boolean false
d-i apt-setup/cdrom/set-failed boolean false
d-i apt-setup/contrib boolean false
d-i apt-setup/non-free boolean false
d-i apt-setup/non-free-firmware boolean false
d-i passwd/root-login boolean false
d-i passwd/user-fullname string %s
d-i passwd/username string %s
d-i passwd/user-password password vmlite
d-i passwd/user-password-again password vmlite
d-i passwd/user-default-groups string sudo
d-i clock-setup/utc boolean true
d-i time/zone string UTC
d-i partman-auto/disk string /dev/vda
d-i partman-auto/method string regular
d-i partman-auto/choose_recipe select atomic
d-i partman-partitioning/confirm_write_new_label boolean true
d-i partman/choose_partition select finish
d-i partman/confirm boolean true
d-i partman/confirm_nooverwrite boolean true
d-i grub-installer/only_debian boolean true
d-i grub-installer/bootdev string /dev/vda
d-i pkgsel/include string openssh-server sudo
d-i pkgsel/upgrade select none
d-i pkgsel/update-policy select none
d-i pkgsel/install-language-support boolean false
d-i popularity-contest/participate boolean false
tasksel tasksel/first multiselect standard, ssh-server
d-i finish-install/reboot_in_progress note
%s
`, in.Hostname, in.SSHUser, in.SSHUser, late)
}

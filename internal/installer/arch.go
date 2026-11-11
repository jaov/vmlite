package installer

import (
	"fmt"
	"strings"

	"vmlite/internal/seed"
)

type archScript struct{}

func (archScript) Type() string { return ArchScript }

func (archScript) Spec(in Input) (Spec, error) {
	if err := validateHostname(in.Hostname); err != nil {
		return Spec{}, err
	}
	if err := validateUser(in.SSHUser); err != nil {
		return Spec{}, err
	}

	script := in.AnswerFile
	if len(script) == 0 {
		script = []byte(defaultArchScript(in))
	} else {
		script = []byte(replacePlaceholders(string(script), in))
	}

	return Spec{
		Type: ArchScript,
		InjectFiles: []seed.File{
			{Name: "vmlite-arch-install.sh", Data: script, Mode: 0o755},
		},
		BootOrder:  []string{"hd", "cdrom"},
		KernelBoot: true,
		KernelPath: "/arch/boot/x86_64/vmlinuz-linux",
		InitrdPath: "/arch/boot/x86_64/initramfs-linux.img",
		KernelArgs: []string{
			"archisobasedir=arch",
			"archisodevice=/dev/sr0",
			"cow_spacesize=1G",
			"script=/run/archiso/bootmnt/vmlite-arch-install.sh",
			"console=ttyS0",
		},
		DiskBus:    "virtio",
		Completion: CompletionShutdown,
	}, nil
}

func defaultArchScript(in Input) string {
	hostname := in.Hostname
	user := in.SSHUser
	key := strings.TrimSpace(in.SSHKey)

	var userSetup string
	switch {
	case user == "root" && key != "":
		userSetup = fmt.Sprintf(`mkdir -p /root/.ssh
echo %s > /root/.ssh/authorized_keys
chmod 700 /root/.ssh
chmod 600 /root/.ssh/authorized_keys
echo 'PermitRootLogin prohibit-password' >> /etc/ssh/sshd_config
`, shellSingleQuote(key))
	case user == "root":
		userSetup = "echo 'root:vmlite' | chpasswd\n"
	case key != "":
		quoted := shellSingleQuote(key)
		userSetup = fmt.Sprintf(`useradd -m -G wheel -s /bin/bash '%s'
echo '%s:vmlite' | chpasswd
mkdir -p /home/%s/.ssh
echo %s > /home/%s/.ssh/authorized_keys
chmod 700 /home/%s/.ssh
chmod 600 /home/%s/.ssh/authorized_keys
chown -R %s:%s /home/%s/.ssh
echo '%%wheel ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-vmlite
chmod 440 /etc/sudoers.d/90-vmlite
`, user, user, user, quoted, user, user, user, user, user, user)
	default:
		userSetup = fmt.Sprintf(`useradd -m -G wheel -s /bin/bash '%s'
echo '%s:vmlite' | chpasswd
echo '%%wheel ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/90-vmlite
chmod 440 /etc/sudoers.d/90-vmlite
`, user, user)
	}

	return fmt.Sprintf(`#!/usr/bin/env bash
set -euxo pipefail

# Wait for the live environment network before pacstrap.
for _ in $(seq 1 90); do
  if curl -fsS --max-time 5 https://archlinux.org >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

DISK=/dev/vda
parted -s "$DISK" mklabel msdos
parted -s "$DISK" mkpart primary ext4 1MiB 100%%
parted -s "$DISK" set 1 boot on
mkfs.ext4 -F "${DISK}1"
mount "${DISK}1" /mnt

pacman -Sy --noconfirm
pacstrap -K /mnt base linux linux-firmware grub openssh sudo dhcpcd
genfstab -U /mnt >> /mnt/etc/fstab

cat > /mnt/root/vmlite-setup.sh <<'VMLITE_EOF'
#!/usr/bin/env bash
set -euxo pipefail
ln -sf /usr/share/zoneinfo/UTC /etc/localtime
hwclock --systohc
echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen
locale-gen
echo 'LANG=en_US.UTF-8' > /etc/locale.conf
echo '%s' > /etc/hostname
mkdir -p /etc/ssh/sshd_config.d
echo 'PasswordAuthentication no' > /etc/ssh/sshd_config.d/99-vmlite.conf
%s
systemctl enable sshd
systemctl enable dhcpcd
sed -i 's|^GRUB_CMDLINE_LINUX_DEFAULT=.*|GRUB_CMDLINE_LINUX_DEFAULT="loglevel=3 quiet console=ttyS0"|' /etc/default/grub
sed -i 's|^#GRUB_TERMINAL_OUTPUT=.*|GRUB_TERMINAL_OUTPUT="console serial"|' /etc/default/grub
grep -q '^GRUB_TERMINAL_OUTPUT=' /etc/default/grub || echo 'GRUB_TERMINAL_OUTPUT="console serial"' >> /etc/default/grub
grep -q '^GRUB_SERIAL_COMMAND=' /etc/default/grub || echo 'GRUB_SERIAL_COMMAND="serial --unit=0 --speed=115200"' >> /etc/default/grub
grub-install --target=i386-pc /dev/vda
grub-mkconfig -o /boot/grub/grub.cfg
VMLITE_EOF

chmod +x /mnt/root/vmlite-setup.sh
arch-chroot /mnt /bin/bash /root/vmlite-setup.sh
umount -R /mnt
sync
poweroff
`, hostname, userSetup)
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

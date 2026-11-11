# Installers and guest sources

`vmlite` supports cloud images, prebuilt disks, and unattended ISO installers.
Cloud images, installer ISOs, and imported disks can use `source.url` with a
required `sha256` and optional cache.

## Linux cloud images

```json
"source": {
  "type": "cloud_image",
  "path": "/var/cache/vmlite/ubuntu-24.04.img"
}
```

Cloud images boot through an overlay. Provisioners can use a generated ed25519
key injected through NoCloud `vendor-data`.

## Ubuntu autoinstall

```json
"source": {
  "type": "iso",
  "path": "/isos/ubuntu-24.04-live-server-amd64.iso",
  "installer": { "type": "ubuntu_autoinstall" }
}
```

- Attaches a `CIDATA` seed with `user-data` and `meta-data`.
- Boots with `autoinstall` through `--location`.
- Generated answer file installs OpenSSH, the configured user, and NOPASSWD
  sudo.
- `vmlite up` starts the installer and returns.
- `vmlite provision` waits for the installer to shut off, ejects the media,
  boots the installed disk, waits for SSH, and runs provisioners.

## Fedora kickstart

```json
"source": {
  "type": "iso",
  "path": "/isos/Fedora-Server-dvd-x86_64-42-1.1.iso",
  "installer": { "type": "fedora_kickstart" }
}
```

- Attaches an `OEMDRV` seed containing `ks.cfg`.
- Anaconda discovers it automatically.
- The default kickstart installs the Fedora Server environment, creates the
  SSH user, installs the generated key, enables sshd, and grants NOPASSWD sudo.
- Uses the same install-completion and finalization path as Ubuntu.

## Debian preseed

```json
"source": {
  "type": "iso",
  "path": "/var/tmp/vmlite-cache/debian-13.7.0-amd64-netinst.iso",
  "installer": { "type": "debian_preseed" }
}
```

- The netinst ISO is remastered with `/preseed.cfg`.
- Boots with `preseed/file=/cdrom/preseed.cfg`.
- Remastering requires `xorriso`.
- `pkgsel` installs `openssh-server` and `sudo` from the guest’s package mirror.

The Debian install/remaster/finalization path was verified. Full SSH
provisioning depends on guest access to a package mirror.

## Arch scripted install

```json
"source": {
  "type": "iso",
  "path": "/isos/archlinux-2026.11.01-x86_64.iso",
  "installer": { "type": "arch_script" }
}
```

- The ISO is remastered with `/vmlite-arch-install.sh`.
- The live ISO is booted with an explicit kernel/initrd and
  `script=/run/archiso/bootmnt/vmlite-arch-install.sh`.
- The default script partitions `/dev/vda`, installs `base`, `linux`,
  `linux-firmware`, `grub`, `openssh`, `sudo`, and `dhcpcd`, configures GRUB
  for BIOS and serial console output, enables `sshd` and `dhcpcd`, then powers
  off.
- The installed system boots from disk, obtains DHCP, and accepts the generated
  SSH key.
- `vmlite up` starts the installer and returns; `vmlite provision` waits for
  shutdown, ejects the media, boots the installed disk, waits for SSH, and runs
  provisioners.

Requirements:

- `firmware: "bios"`; direct kernel boot is not implemented for UEFI.
- Guest network access to Arch package mirrors.
- `xorriso` on the host for remastering.
- At least 2 GiB of guest memory. The live environment uses a 1 GiB cowspace.

Custom scripts can be supplied with `installer.inline` or `installer.config`
and use the usual placeholders:

- `{{VMLITE_HOSTNAME}}`
- `{{VMLITE_SSH_USER}}`
- `{{VMLITE_SSH_AUTHORIZED_KEY}}`

The Arch installer was verified end to end on this host.

## Windows autounattend

```json
"source": {
  "type": "iso",
  "path": "/isos/windows-server.iso",
  "installer": {
    "type": "windows_autounattend",
    "windows": { "image_index": 1 }
  }
}
```

Generated defaults include:

- UEFI/GPT or BIOS/MBR disk layout
- image index selection
- Administrator password
- OpenSSH Server capability installation
- `administrators_authorized_keys` with strict ACLs
- SSH firewall rule
- OOBE suppression

`ssh.user` defaults to `Administrator`. Install completion is SSH-based:
`vmlite provision` waits for the installed Windows guest instead of waiting for
the installer to shut off.

If `windows.admin_password` is omitted, vmlite generates a password and writes
it to `.vmlite/<name>/windows-admin-password` (`0600`). `up` and `status` print
the path.

Windows 11 may require:

```json
"firmware": "uefi",
"secure_boot": true,
"tpm": { "version": "2.0" }
```

`disk.bus: "sata"` avoids virtio driver injection during Setup.

For custom media, supply `installer.config` with placeholders:

- `{{VMLITE_HOSTNAME}}`
- `{{VMLITE_SSH_USER}}`
- `{{VMLITE_SSH_AUTHORIZED_KEY}}`

The generated default targets Windows Server evaluation media. It uses
`Add-WindowsCapability` for OpenSSH, which needs the capability payload or
Windows Update.

> **Status:** Windows install and WinPE paths are implemented and unit-tested,
> but have not been exercised end to end on this host. A practical Windows
> guest needs a 30–64 GiB disk; this environment lacks the space. The gated
> tests skip when media or space is unavailable.

## Windows PE

WinPE is a small Microsoft boot image for CI. Build it on Windows with the
Windows ADK + WinPE add-on:

```powershell
.\scripts\build-winpe.ps1 -Output .\artifacts\winpe-vmlite.iso
```

`scripts/winpe/startnet.cmd` writes a marker to a FAT data disk and calls
`wpeutil shutdown`. The Linux test verifies the marker:

```bash
VMLITE_TEST_WINPE=1 \
VMLITE_TEST_WINPE_ISO=/artifacts/winpe-vmlite.iso \
  go test -tags integration -run TestWindowsPEBoot -v ./...
```

WinPE validates Windows boot and media plumbing without a full installation.

## Manual ISO and prebuilt disk provisioning

FreeBSD, OpenBSD, NetBSD, or any other guest can be provisioned after a manual
install or from a prebuilt disk. Arch also has the automated `arch_script`
installer above.

### Manual ISO

```json
"source": { "type": "iso", "path": "/isos/openbsd.iso" },
"ssh": {
  "user": "deploy",
  "key": "/home/me/.ssh/id_ed25519"
},
"provisioners": [
  { "type": "shell", "path": "./provision/setup.sh" }
]
```

- `ssh.key` is required; vmlite does not generate a key it cannot deliver.
- `vmlite up` starts the installer and never provisions it.
- Install the OS, authorize the key, then run `vmlite provision`.

### Prebuilt disk

```json
"source": { "type": "disk", "path": "/images/ubuntu-postgres.qcow2" },
"ssh": {
  "user": "deploy",
  "key": "/home/me/.ssh/id_ed25519"
},
"provisioners": [
  { "type": "shell", "path": "./provision/setup.sh" }
]
```

- `ssh.key` is required.
- `vmlite up` starts the disk and auto-provisions because the OS is already
  installed.
- Use `ssh.user: "root"` for guests without sudo; root provisioning skips
  `sudo`.

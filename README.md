# vmlite

A small, dependency-free Go CLI that creates local KVM/QEMU guests through
`virt-install` and `virsh`.

## Install on a Linux host

vmlite runs on Linux hosts with KVM, libvirt, and virt-install. FreeBSD is
supported as a **guest**, not as a vmlite host.

```bash
./install.sh --dry-run   # build a temporary vmlite and show the plan
./install.sh             # install dependencies and vmlite
```

`install.sh` is now a small bootstrap: it builds a temporary vmlite binary and
runs `vmlite install`, or uses a prebuilt `./vmlite` when Go is unavailable.
The Go installer handles Debian/Ubuntu, Fedora/RHEL-family, Arch, and openSUSE
package managers, installs QEMU/libvirt/virt-install/ISO/SSH tools, checks
`/dev/kvm`, starts the default libvirt network, and adds the invoking user to
the `libvirt`/`kvm` groups.

Options are forwarded to the Go installer:

```bash
vmlite install --dry-run
vmlite install --binary ./vmlite               # prebuilt local binary
vmlite install --binary-url URL --binary-sha256 SHA256
vmlite install --no-deps                       # skip packages
vmlite install --prefix /usr/bin
vmlite doctor                                  # read-only readiness check
```

## Usage

```bash
vmlite up                 # uses ./vm.json; `vmlite` is an alias for up
vmlite up path/to/vm.json
vmlite --connect qemu:///session vm.json
vmlite status path/to/vm.json
vmlite destroy --force path/to/vm.json
```

Lifecycle state and generated VM assets live in `.vmlite/` next to the
configuration file. `up` is idempotent: an unchanged running VM is a no-op,
a stopped VM is started, and config/connection drift requires `--recreate`.
`destroy` refuses to remove a running VM unless `--force` is passed.

### Shell provisioners

```json
"ssh": { "user": "vmlite" },
"provisioners": [
  { "type": "shell", "path": "./provision/postgres-tomcat.sh", "run": "once" },
  { "type": "shell", "inline": "echo refreshed >> /var/tmp/provisioned", "run": "always" }
]
```

- `up` generates an ed25519 key under `.vmlite/`, injects the public key through
  a NoCloud `vendor-data` cloud-config, waits for the NAT guest IP and SSH, then
  runs the provisioners as root.
- `run: "once"` skips unchanged scripts; `run: "always"` reruns on every
  `vmlite provision`.
- `vmlite provision vm.json` reruns pending/always scripts.
- `vmlite provision --force vm.json` reruns once-provisioners too.
- Shell provisioners require a Linux cloud image, an automated ISO installer,
  a manual ISO with `ssh.key`, or a prebuilt disk with `ssh.key`, on
  `qemu:///system` with a guest-visible network. `session`/user-mode
  networking is rejected.
- Manual ISO installs and prebuilt disks require a user-provided `ssh.key`;
  vmlite does not generate a key that the guest cannot receive. `up` starts a
  manual ISO and never provisions it. After installing the OS and authorizing
  the key, run `vmlite provision`. Disk sources auto-provision on `up` because
  the OS is already installed.
- `vmlite status` reports the guest IP; services are reached at
  `guest-ip:5432`, `guest-ip:8080`, etc.

### Automated ISO installers

An ISO source can carry an unattended installer spec:

```json
{
  "name": "ubuntu-auto",
  "memory": 2048,
  "cpus": 2,
  "source": {
    "type": "iso",
    "path": "/var/tmp/ubuntu-24.04.3-live-server-amd64.iso",
    "installer": { "type": "ubuntu_autoinstall" }
  },
  "disk": { "size": 8 },
  "network": { "mode": "default" },
  "graphics": { "type": "none" },
  "provisioners": [
    { "type": "shell", "inline": "apt-get update && apt-get install -y nginx" }
  ]
}
```

- `vmlite up` starts the installer and returns; it never provisions the
  installer environment.
- `vmlite provision` waits for the installer to finish, ejects its CD-ROM,
  boots the installed disk, waits for SSH, then runs the shell provisioners.
- `installer.config` may point at a custom answer file. Use
  `{{VMLITE_HOSTNAME}}`, `{{VMLITE_SSH_USER}}`, and
  `{{VMLITE_SSH_AUTHORIZED_KEY}}` placeholders to receive the generated key.
- Without `installer.config`, vmlite generates a minimal answer file that
  installs SSH, the configured user, and NOPASSWD sudo.
- Automated installers require `qemu:///system` and guest-visible networking.
- The installer boot order is `hd,cdrom`, so a reboot after installation boots
  the disk instead of restarting the installer.

Fedora Server uses kickstart answer media:

```json
"source": {
  "type": "iso",
  "path": "/var/tmp/Fedora-Server-dvd-x86_64-42-1.1.iso",
  "installer": { "type": "fedora_kickstart" }
}
```

The generated `ks.cfg` is placed on an `OEMDRV` volume, which Anaconda detects
automatically. The default kickstart installs the Fedora Server environment,
creates the SSH user, installs the generated key with `sshkey`, enables sshd,
and grants NOPASSWD sudo.

### Windows PE boot tests

Windows client ISOs are too large for the default CI disk budget, so the first
Windows target is a small WinPE image built from the Windows ADK:

```powershell
# On an elevated Windows machine with the ADK + WinPE add-on:
.\scripts\build-winpe.ps1 -Output .\artifacts\winpe-vmlite.iso
```

The custom `scripts/winpe/startnet.cmd` initializes WinPE, formats the second
disk, writes `vmlite-winpe-ok.txt`, and powers the VM off. The Linux test then
verifies the marker:

```bash
VMLITE_TEST_WINPE=1 \
VMLITE_TEST_WINPE_ISO=/artifacts/winpe-vmlite.iso \
  go test -tags integration -run TestWindowsPEBoot -v ./...
```

This validates Windows boot, CD-ROM attachment, and the same plan/step command
primitives used elsewhere. It does not exercise Windows Setup or SSH
provisioning; those need a full Windows evaluation ISO and a larger disk,
for example 30–64 GiB.

Debian uses preseed:

```json
"source": {
  "type": "iso",
  "path": "/var/tmp/vmlite-cache/debian-13.7.0-amd64-netinst.iso",
  "installer": { "type": "debian_preseed" }
}
```

The netinst ISO is remastered with `/preseed.cfg`, and the installer boots with
`preseed/file=/cdrom/preseed.cfg`. Remastering requires `xorriso` in addition
to the ISO creation tools. Debian’s `pkgsel` installs `openssh-server` and
`sudo` from the configured network mirror, so the installer guest needs
outbound access to `deb.debian.org` (or a preseed pointing at a reachable
mirror).

A cloud-image example:

```json
{
  "name": "dev-vm",
  "memory": 2048,
  "cpus": 2,
  "os_variant": "ubuntu24.04",
  "firmware": "bios",
  "source": {
    "type": "cloud_image",
    "path": "/var/cache/vmlite/ubuntu.qcow2",
    "cloud_init": "/var/tmp/user-data"
  },
  "disk": { "size": 10, "format": "qcow2" },
  "mounts": [
    { "source": "/var/tmp/shared", "target": "shared_data", "type": "9p" }
  ],
  "network": { "mode": "auto" },
  "graphics": { "type": "none" }
}
```

Sources include `none`, `cloud_image`, `install_tree`, `iso`, `disk`, and
`pxe`. See `TESTING.md` for the full configuration and distro support matrix.

For reproducible cloud images, use `source.url` with a required `source.sha256`
instead of `source.path`:

```json
"source": {
  "type": "cloud_image",
  "url": "https://cloud-images.ubuntu.com/minimal/releases/noble/release/ubuntu-24.04-minimal-cloudimg-amd64.img",
  "sha256": "<64-character digest>",
  "cache": "/var/tmp/vmlite-cache",
  "cloud_init": "./cloud-init.yaml"
}
```

Images are cached under `~/.cache/vmlite` by default or `source.cache`. Every
use verifies the checksum. `vmlite up --offline` refuses to download a missing
image. `destroy` keeps the image cache so recreating is fast and reproducible.

## Test

```bash
go test ./...
go test -tags integration -v ./...
```

Boot profiles, installer-ISO checks, and the self-hosted nested test are gated
by environment variables documented in `TESTING.md`.

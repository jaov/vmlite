# vmlite

A small, dependency-free Go CLI that creates local KVM/QEMU guests through
`virt-install` and `virsh`.

## Install on a Linux host

vmlite runs on Linux hosts with KVM, libvirt, and virt-install. FreeBSD is
supported as a **guest**, not as a vmlite host.

```bash
./install.sh --dry-run   # show what would change
./install.sh             # install dependencies and build vmlite
```

The installer handles Debian/Ubuntu, Fedora/RHEL-family, Arch, and openSUSE
package managers. It installs QEMU, libvirt, virt-install, and ISO tools,
checks `/dev/kvm`, starts the default libvirt network when present, and adds
the invoking user to the `libvirt`/`kvm` groups.

Options:

```bash
./install.sh --binary ./vmlite      # install a prebuilt static binary
./install.sh --no-deps              # skip package installation
./install.sh --prefix /usr/bin      # choose the install prefix
```

## Usage

```bash
vmlite                 # uses ./vm.json
vmlite path/to/vm.json
vmlite --connect qemu:///session vm.json
```

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

## Test

```bash
go test ./...
go test -tags integration -v ./...
```

Boot profiles, installer-ISO checks, and the self-hosted nested test are gated
by environment variables documented in `TESTING.md`.

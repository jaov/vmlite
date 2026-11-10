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

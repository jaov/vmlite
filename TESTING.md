# Testing vmlite

## Unit tests

Unit tests need no libvirt and are safe to run anywhere:

```bash
go test ./...
```

They cover config parsing/validation, argument construction, overlay/seed
generation, image tool checks, URI-independent preflight logic (using fake
`virsh`/`virt-install`/ISO tools), and CLI usage errors. Tests that require
`/dev/kvm` skip automatically.

## Integration tests

Integration tests create real libvirt domains, inspect their XML, and remove
them again. They are behind a build tag:

```bash
go test -tags integration -v ./...
```

Defaults:

- `/dev/kvm`, `virt-install`, and `virsh` must be available.
- Tests use `qemu:///session` unless `VMLITE_CONNECT` is set.
- Each VM uses a 1 GiB disk, 256 MiB RAM, and no OS install, so a run takes a
  few seconds.
- Mount sources are created under `/tmp` with mode `0755` so that the system
  qemu user can read them.

### Environment variables

| Variable | Meaning |
|---|---|
| `VMLITE_CONNECT` | libvirt URI used by the tests (default `qemu:///session`) |
| `VMLITE_TEST_SYSTEM=1` | also run the system-mode XML test against `qemu:///system` |
| `VMLITE_TEST_KEEP=1` | keep domains and disks after a test for debugging |

Run both session and system mode:

```bash
VMLITE_TEST_SYSTEM=1 go test -tags integration -v ./...
```

## Bootable guest tests

`TestBootableGuest` boots a real cloud image, waits for SSH, copies a marker
file with scp, and then verifies the 9p mount for profiles that support it. It
is opt-in because it needs an image and takes about 30-40 seconds.

### Get an image

```bash
./scripts/fetch-cloud-image.sh
```

This downloads the Ubuntu 24.04 minimal cloud image (~250 MiB) to
`~/.cache/vmlite/`. You can also pass a destination path:

```bash
./scripts/fetch-cloud-image.sh /var/tmp/ubuntu-24.04.qcow2
```

Tested images:

| Distro | Profile | Image | Boot test |
|---|---|---|---|
| Ubuntu 24.04 | `ubuntu` | `ubuntu-24.04-minimal-cloudimg-amd64.img` | SSH + scp + 9p |
| Debian 12 | `debian` | `debian-12-generic-amd64.qcow2` | SSH + scp + 9p |
| Fedora 42 | `fedora` | Cloud Base Generic | SSH + scp + 9p |
| Arch | `arch` | official cloudimg | SSH + scp + 9p |
| FreeBSD 15.1 | `freebsd` | BASIC-CLOUDINIT ufs | profile defined, not yet verified |

Debian has two image flavours. Use **generic**, not genericcloud:

```bash
curl -fL -o ~/.cache/vmlite/debian-12-generic-amd64.qcow2 \
  https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2

VMLITE_TEST_PROFILE=debian \
VMLITE_TEST_IMAGE="$HOME/.cache/vmlite/debian-12-generic-amd64.qcow2" \
  go test -tags integration -run TestBootableGuest -v ./...
```

The `genericcloud` image uses Debian's cloud kernel, which has
`# CONFIG_NET_9P is not set`; 9p mounts cannot work there. The **generic** image
uses the standard kernel, includes cloud-init, and passes the full SSH + 9p
boot test.

### Run the boot test

```bash
VMLITE_TEST_PROFILE=ubuntu \
VMLITE_TEST_IMAGE="$HOME/.cache/vmlite/ubuntu-24.04-minimal-cloudimg-amd64.img" \
  go test -tags integration -run TestBootableGuest -v ./...
```

Requirements:

- `/dev/kvm`, system `qemu:///system`, `qemu-img`, `ssh`, `scp`, `ssh-keygen`,
  and `xorrisofs`/`genisoimage`/`mkisofs`.
- At least 3 GiB free in the test work directory (default `/var/tmp`).
- The test pre-creates a qemu-writable overlay and stages the base image so the
  `libvirt-qemu` user can read them.

Environment variables:

| Variable | Meaning |
|---|---|
| `VMLITE_TEST_PROFILE` | `ubuntu`, `debian`, `fedora`, `arch`, or `freebsd` (default `ubuntu`) |
| `VMLITE_TEST_IMAGE` | path to the qcow2 cloud image (required) |
| `VMLITE_TEST_OS_VARIANT` | override the profile's `--os-variant` |
| `VMLITE_TEST_SSH_USER` | override the guest user created by the seed |
| `VMLITE_TEST_WORKDIR` | work directory (default `/var/tmp`) |
| `VMLITE_TEST_KEEP=1` | keep the domain, overlay, seed, and work dir |

The guest check detects `uname -s`, copies a marker file over scp, waits for
cloud-init when the profile expects it, and mounts the shared directory using
the profile's mount style (`linux9p`, `freebsd9p`, or none).

### Why system mode?

Session-mode SSH was attempted with passt and a `portForward` for
`127.0.0.1:<port>` → guest `22`. On this host (libvirt 10.0, passt
`0.0~git20240220`), libvirt's passt child segfaults on startup, with or without
port forwarding, and slirp rejects `portForward` outright. The boot test
therefore uses system mode with the default NAT network and DHCP.

## VM configuration (v2)

Image-based VM example:

```json
{
  "name": "dev-vm",
  "memory": 2048,
  "cpus": 2,
  "os_variant": "ubuntu24.04",
  "firmware": "bios",
  "source": {
    "type": "cloud_image",
    "path": "/var/cache/vmlite/ubuntu-24.04-minimal-cloudimg-amd64.img",
    "cloud_init": "/var/tmp/dev-vm-user-data"
  },
  "disk": {
    "size": 10,
    "path": "/var/tmp/dev-vm-overlay.qcow2",
    "format": "qcow2"
  },
  "mounts": [
    { "source": "/var/tmp/shared", "target": "shared_data", "type": "9p" }
  ],
  "network": { "mode": "auto" },
  "graphics": { "type": "none" }
}
```

`source.type` selects how the guest boots:

| Type | Meaning |
|---|---|
| `none` | Empty disk, no installation; define and start the VM |
| `cloud_image` | qcow2/raw base image with an overlay; optional cloud-init seed |
| `install_tree` | Linux install tree URL; optional `extra_args` |
| `iso` | Installer ISO; pair with `graphics: "vnc"` or a serial console |
| `disk` | Import an existing installed disk |
| `pxe` | Network boot |

Other fields:

- `firmware`: `bios` (default) or `uefi`.
- `disk.size` is GiB. `disk.path` is optional. `disk.format` accepts `auto`,
  `qcow2`, or `raw`. `auto` detects base/source images with `qemu-img`; newly
  created empty disks default to qcow2.
- `extra_disks[].format` also accepts `auto`; qemu-img detects the media format.
  `qemu-img` must be installed when a format is auto-detected.
- `mounts[].type`: `9p` (default), `none`, or `virtiofs` (reserved; currently
  rejected).
- `network.mode`: `auto`, `default`, `user`, or `bridge` (with `network.bridge`).
- `graphics.type`: `none` (default), `vnc`, or `spice`; `graphics.listen`
  defaults to `127.0.0.1`.
- `autoconsole` defaults to false. Set it to true when you want virt-install to
  attach to the guest console and block.
- `extra_disks` attaches additional media, for example installer config discs.

Cloud-image behavior:

- `disk.size` is the overlay's virtual size. Create it larger than the base
  image so cloud-init's growpart can expand the root filesystem.
- An explicit `disk.path` is reused as a user-managed disk. An implicit default
  path (`<image-dir>/<name>-overlay.qcow2`) that already exists is an error.
- `source.cloud_init` is a user-data file. vmlite generates a persistent
  NoCloud seed ISO next to the overlay and adds the SMBIOS `ds=nocloud` hint.
- vmlite sets `on_reboot=restart`/`on_crash=restart` so a guest reboot does not
  destroy the domain.

In system mode, the base image must be readable and the overlay writable by the
qemu user (`libvirt-qemu`). Session mode avoids those permissions issues; use
`--connect qemu:///session` when needed.

## Inspecting leftovers

Cleanup normally leaves nothing behind. To check:

```bash
virsh -c qemu:///session list --all
virsh -c qemu:///system list --all
```

Leftover domains are named `vmlite-it-<pid>-<n>` (or `vmlite-it-boot-<...>`)
and can be removed with:

```bash
virsh -c <uri> undefine <name> --remove-all-storage
```

## Caveats

- The sample `vm.json` mounts `./data` from this repository. In system mode,
  `/home/<user>` is typically not traversable by the `libvirt-qemu` user, so use
  `--connect qemu:///session` with the sample or point the mount at a path
  readable by qemu (for example under `/tmp`).
- Only a few GiB may be free on `/`; keep test disks small.
- On CI runners without `/dev/kvm`, integration tests skip automatically.

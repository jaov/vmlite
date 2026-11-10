# Troubleshooting

## Start with validate and doctor

```bash
vmlite validate vm.json
vmlite doctor
```

`validate` checks configuration and local paths without changing anything.
`doctor` checks host readiness: KVM, libvirt, virt-install, ISO tools, SSH
tools, the default network, and the installed binary.

## Provisioners fail on session mode

Shell/SSH provisioners require a guest-visible network. Use system libvirt and
the default NAT network:

```bash
vmlite up --connect qemu:///system vm.json
```

Session mode uses QEMU user networking, which has no host-visible guest IP.
Provisioners are rejected in that mode.

## qemu cannot read the project directory

System-mode `qemu` runs as a different user. For testing or projects under a
home directory with restrictive permissions, use a qemu-accessible state
directory:

```bash
vmlite up --state-dir /tmp/vmlite-state vm.json
```

or place disks, ISOs, seeds, and mounts under a world-readable directory such
as `/tmp` or `/var/tmp`.

## Config or connection changed

`up` refuses to silently change a defined domain:

```text
config or connection changed for "dev"; run destroy or use --recreate
```

Use:

```bash
vmlite up --recreate vm.json
```

Provisioner scripts and SSH settings do not participate in the VM-shape hash,
so changing a script does not require recreation.

## `destroy` refuses because the VM is running

```bash
vmlite destroy --force vm.json
```

`destroy` removes the managed domain, disks, seeds, and asset directory. The
image cache is kept.

## Status exits 3

`vmlite status` returns `3` when the VM is not running, missing, or not
created. Use `--json` for scripts:

```bash
vmlite status --json vm.json || true
```

The JSON document is still emitted with `running: false`.

## Missing host tools

Install or check tools:

```bash
vmlite install --dry-run
vmlite install
vmlite doctor
```

Common requirements:

- `qemu-img` for cloud images and auto-detected disk formats
- `virt-install`, `virsh`
- `xorriso` / `xorrisofs` for seed media and ISO remastering
- `ssh`, `scp`, `ssh-keygen`
- `mkfs.vfat` and `7z` only for the WinPE test

## Image download or checksum errors

URL cloud images require a SHA-256. The cache is verified on each use.
`--offline` refuses to download a missing image.

```bash
vmlite up --offline vm.json
```

Remove a corrupted cache entry and retry, or point `source.cache` at a larger
filesystem.

## Installer sits at a prompt or does not finish

Check:

- the answer file is accessible (`vmlite validate`)
- the installer media is the expected edition
- the guest has network access if the installer downloads packages
- the guest has enough disk space
- `vmlite status --json` for the install phase

For Linux/BSD installers, `provision` waits for the installer domain to shut
off, ejects media, clears the direct kernel boot, and starts the installed disk.
If the domain stays in `in shutdown`, `provision` waits through the transition.

For Windows, completion is SSH-based. Use a longer timeout if the install is
slow:

```bash
vmlite provision --timeout 60m vm.json
```

## Windows tests are not run here

Windows install and WinPE paths are implemented and unit-tested, but have not
been exercised end to end on the development host. A practical Windows guest
needs 30–64 GiB; this environment lacks the free space. The full Windows test
skips unless `VMLITE_TEST_WORKDIR` has about 40 GiB free.

## 9p mounts are missing

- Ensure the guest kernel has 9p support.
- Debian `genericcloud` lacks it; use the generic cloud image.
- FreeBSD exposes `virtio_p9fs` but the test does not assert a mount.
- `virtiofs` is not implemented yet.

## Stale DHCP leases after an installer reboot

`vmlite` prefers the active ARP lease over the DHCP lease list, so an installed
guest that gets a new address is still found. If status shows an old IP, rerun
`vmlite provision`; it re-resolves the address.

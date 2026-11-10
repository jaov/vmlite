# vmlite

A small, dependency-free Go CLI that creates local KVM/QEMU guests through
`virt-install` and `virsh`.

vmlite is Vagrant-like without the Ruby dependency:

- project-local `.vmlite/` state
- reproducible cloud images and installer media
- `up`, `provision`, `status`, `destroy`, `validate`
- shell and PowerShell provisioners
- automated Ubuntu, Fedora, Debian, and Windows installation paths

## Install on a Linux host

vmlite runs on Linux hosts with KVM, libvirt, and virt-install. FreeBSD is
supported as a **guest**, not as a vmlite host.

```bash
./install.sh --dry-run   # build a temporary vmlite and show the plan
./install.sh             # install dependencies and vmlite
```

`install.sh` is a bootstrap: it builds a temporary vmlite binary and runs
`vmlite install`, or uses a prebuilt `./vmlite` when Go is unavailable.

The Go installer handles Debian/Ubuntu, Fedora/RHEL-family, Arch, and openSUSE
package managers, installs QEMU/libvirt/virt-install/ISO/SSH tools, checks
`/dev/kvm`, starts the default libvirt network, and adds the invoking user to
the `libvirt`/`kvm` groups.

Options are forwarded to the Go installer:

```bash
vmlite install --dry-run
vmlite install --binary ./vmlite
vmlite install --binary-url URL --binary-sha256 SHA256
vmlite install --no-deps
vmlite install --prefix /usr/bin
vmlite doctor                  # read-only readiness check
```

## Quick start

```bash
vmlite up                     # uses ./vm.json; `vmlite` is an alias for up
vmlite up path/to/vm.json
vmlite status path/to/vm.json
vmlite status --json vm.json
vmlite provision vm.json
vmlite destroy --force vm.json
```

A cloud-image configuration:

```json
{
  "name": "dev",
  "memory": 2048,
  "cpus": 2,
  "os_variant": "ubuntu24.04",
  "firmware": "bios",
  "source": {
    "type": "cloud_image",
    "path": "/var/cache/vmlite/ubuntu-24.04.img"
  },
  "disk": { "size": 10, "format": "qcow2" },
  "network": { "mode": "auto" },
  "graphics": { "type": "none" }
}
```

State and generated assets live in `.vmlite/` next to the config, or under
`--state-dir`. `up` is idempotent: an unchanged running VM is a no-op, a
stopped VM is started, and config/connection drift requires `--recreate`.
`destroy` refuses to remove a running VM unless `--force` is passed.

Validate before creating anything:

```bash
vmlite validate vm.json
# vm.json: valid
```

## Commands

| Command | Purpose |
|---|---|
| `up` | create or start the VM (default command) |
| `provision` | run or rerun provisioners; finish automated installers |
| `status` | show VM, IP, installer, and provisioner state |
| `destroy` | remove the VM and managed files |
| `validate` | validate config and local paths without changes |
| `install` | install host dependencies and vmlite |
| `doctor` | check host readiness |

See [docs/cli.md](docs/cli.md) for flags, examples, JSON output, and exit
codes.

## Provisioning

```json
"ssh": { "user": "vmlite" },
"provisioners": [
  { "type": "shell", "path": "./provision/postgres.sh", "run": "once" },
  { "type": "shell", "inline": "echo refreshed >> /var/tmp/log", "run": "always" },
  { "type": "powershell", "inline": "Set-Content C:\\vmlite-ok.txt 'ready'" }
]
```

- `shell` runs `bash -s` with `sudo -n` for non-root users.
- `powershell` runs `powershell -NoProfile -NonInteractive -Command -` without
  sudo.
- `once` records the script SHA-256 and skips unchanged scripts.
- `always` reruns on every `vmlite provision`.
- `--force` reruns once-provisioners.
- `--timeout 45m` overrides guest readiness waits.

## Sources and installers

| Source | Use |
|---|---|
| `cloud_image` | Ubuntu, Debian, Fedora, Arch, FreeBSD cloud images |
| `iso` | manual or automated installers |
| `disk` | imported/prebuilt installed disk |
| `install_tree` | virt-install location install |
| `pxe` | network boot |
| `none` | blank VM |

Automated ISO installers:

| Installer | Media |
|---|---|
| `ubuntu_autoinstall` | Ubuntu live-server ISO |
| `fedora_kickstart` | Fedora Server ISO |
| `debian_preseed` | Debian netinst ISO |
| `windows_autounattend` | Windows Server/client ISO |

Manual ISO and prebuilt disk sources are provisionable with a user-provided
`ssh.key`. This covers Arch, FreeBSD, OpenBSD, NetBSD, and other guests today.

See:

- [docs/configuration.md](docs/configuration.md) — full JSON field reference
- [docs/installers.md](docs/installers.md) — per-installer details and limitations

## Windows status

The Windows autounattend and WinPE paths are implemented and unit-tested, but
have **not** been exercised end to end on this host. A practical Windows guest
needs a 30–64 GiB disk, which this development environment does not have. The
Windows integration tests are opt-in and skip when media or space is
unavailable.

## Documentation

- [CLI reference](docs/cli.md)
- [Configuration reference](docs/configuration.md)
- [Installers and guest sources](docs/installers.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Testing guide](TESTING.md)

## Test

```bash
go test ./...
go test -tags integration -v ./...
```

Boot profiles, installer tests, and the self-hosted nested test are gated by
environment variables documented in [TESTING.md](TESTING.md).

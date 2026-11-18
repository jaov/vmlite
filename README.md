# vmlite

A small, dependency-free Go CLI that creates local KVM/QEMU guests through
`virt-install` and `virsh`.

vmlite is Vagrant-like without the Ruby dependency:

- project-local `.vmlite/` state
- reproducible cloud images and installer media
- `up`, `provision`, `status`, `destroy`, `validate`
- shell and PowerShell provisioners
- automated Ubuntu, Fedora, Debian, Arch, and Windows installation paths

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

### Release packages

GitHub Releases also include `.deb` and `.rpm` packages:

```bash
sudo apt install ./vmlite_<version>-1_amd64.deb
sudo dnf install ./vmlite-<version>-1.x86_64.rpm
```

Packages install `vmlite` to `/usr/bin` with shell completions and a man page.
They are release assets, not a hosted APT/DNF repository. If a manual install
exists in `/usr/local/bin`, that copy takes precedence in `PATH`.

Package users should not run `vmlite install`; the package manager owns the
files. Use `vmlite doctor` to check host readiness.

## Quick start

```bash
vmlite                        # show help
vmlite init                   # write a starter vm.json from the cloud-image example
vmlite up                     # edit source.path first
vmlite up path/to/vm.json
vmlite status path/to/vm.json
vmlite status --json vm.json
vmlite ssh vm.json
vmlite ssh --command 'uname -a' vm.json
vmlite scp ./app.conf /etc/app.conf vm.json
vmlite list
vmlite stop vm.json
vmlite start vm.json
vmlite restart vm.json
vmlite console vm.json
vmlite provision vm.json
vmlite destroy --force vm.json
```

`vmlite init` writes a starter configuration. Use `--template minimal` for a
runnable blank VM or `--list` to see all templates.

All VM commands accept a managed VM name or a config path. Once a VM exists,
`vmlite status dev`, `vmlite ssh dev`, and `vmlite destroy dev` resolve
`<state-dir>/dev.json`, so the config file is not required. State is discovered
from `--state-dir`, `VMLITE_STATE_DIR`, `state_dir` in the config, or `.vmlite`
next to the config.

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
    "url": "https://cloud-images.ubuntu.com/minimal/releases/noble/release-20261017/ubuntu-24.04-minimal-cloudimg-amd64.img",
    "sha256": "b3296484e362ba2e713489a5de6225c719d9500962bde25d15f2f21e512c7658"
  },
  "disk": { "size": 10, "format": "qcow2" },
  "network": { "mode": "auto" },
  "graphics": { "type": "none" }
}
```

State and generated assets live in `.vmlite/` next to the config, or under
`--state-dir`, `VMLITE_STATE_DIR`, or `state_dir` in the config. `up` is
idempotent: an unchanged running VM is a no-op, a stopped VM is started, and
config/connection drift requires `--recreate`. `destroy` refuses to remove a
running VM unless `--force` is passed.

Validate before creating anything:

```bash
vmlite validate vm.json
# vm.json: valid
```

## Commands

| Command | Purpose |
|---|---|
| `up` | create or start the VM |
| `init` | write a starter `vm.json` from an example template |
| `provision` | run or rerun provisioners; finish automated installers |
| `status` | show VM, IP, installer, and provisioner state |
| `inspect` | show detailed VM, disk, network, and libvirt state |
| `start` | start a stopped VM by name or config |
| `stop` | gracefully stop a running VM by name or config |
| `restart` | reboot or start a VM by name or config |
| `ssh` | open a shell or run a guest command |
| `scp` | copy a host file or directory into the guest |
| `console` | attach to serial or print VNC details |
| `list` | list managed VMs in the state directory |
| `destroy` | remove the VM and managed files |
| `validate` | validate config and local paths without changes |
| `schema` | print the vmlite JSON Schema |
| `install` | install host dependencies and vmlite |
| `doctor` | check host readiness |
| `version` | print the vmlite version |
| `help` | show top-level or command-specific help |

See [docs/cli.md](docs/cli.md) for flags, examples, JSON output, and exit
codes.

### Guest access

Guest access uses the managed state IP and SSH key:

```bash
vmlite ssh vm.json
vmlite ssh --command 'systemctl status nginx' vm.json
vmlite scp ./app.conf /etc/app.conf vm.json
vmlite scp --recursive ./assets /opt/assets vm.json
vmlite console vm.json              # graphical details, or serial if no display
vmlite console --type serial vm.json
```

SSH and scp require `qemu:///system` with a guest-visible network and a
managed SSH key. `stop` uses ACPI shutdown and waits; `--force` destroys the VM
if graceful shutdown times out.

## Provisioning

```json
"ssh": { "user": "vmlite" },
"provisioners": [
  {
    "type": "shell",
    "path": "./provision/postgres.sh",
    "run": "once",
    "uploads": [
      { "source": "./config/app.conf", "target": "/etc/app.conf" }
    ],
    "env": { "APP_ENV": "dev" }
  },
  { "type": "shell", "inline": "echo refreshed >> /var/tmp/log", "run": "always" }
]
```

- `shell` runs `bash -s` with `sudo -n` for non-root users.
- `powershell` runs `powershell -NoProfile -NonInteractive -Command -` without
  sudo. Use it for Windows guests.
- `once` records the script SHA-256 and skips unchanged scripts.
- `always` reruns on every `vmlite provision`.
- `--force` reruns once-provisioners.
- `--timeout 45m` overrides guest readiness waits.
- `uploads` copies files into the guest before the script runs.
- `env` reaches shell provisioners through `env` and is emitted as
  `$env:NAME = 'value'` for PowerShell.

Windows example:

```json
"provisioners": [
  { "type": "powershell", "inline": "Set-Content C:\\vmlite-ok.txt 'ready'" }
]
```

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
| `arch_script` | Arch Linux ISO |
| `windows_autounattend` | Windows Server/client ISO |

Manual ISO and prebuilt disk sources are provisionable with a user-provided
`ssh.key`. This covers FreeBSD, OpenBSD, NetBSD, and other guests today.

See:

- [docs/configuration.md](docs/configuration.md) — full JSON field reference
- [docs/installers.md](docs/installers.md) — per-installer details and limitations

## Windows status

The Windows autounattend and WinPE paths are implemented and unit-tested, but
have **not** been exercised end to end on this host. A practical Windows guest
needs a 30–64 GiB disk, which this development environment does not have. The
Windows integration tests are opt-in and skip when media or space is
unavailable.

## Version and releases

```bash
vmlite version
vmlite --version
```

Local builds can set the version explicitly:

```bash
go build -ldflags "-X main.version=v0.1.0" -o vmlite .
```

CI runs formatting, `go vet`, `go test -race`, and a build on every push and
pull request. Tags matching `v*` build `vmlite-linux-amd64`, generate a
SHA-256 file, and publish a GitHub release.

## Documentation

- [CLI reference](docs/cli.md)
- [Configuration reference](docs/configuration.md)
- [JSON Schema](schema/vmlite.schema.json)
- [Example configurations](examples/)
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

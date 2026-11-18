# vmlite CLI

`vmlite` creates and manages local KVM/libvirt guests from a JSON configuration
file. State is project-local under `.vmlite/`.

## Synopsis

```text
vmlite [command] [options] [config.json|name]
```

`config.json` defaults to `vm.json`. Running `vmlite` with no command prints
help; it does not start a VM.

State is discovered in this order:

1. `--state-dir`
2. `VMLITE_STATE_DIR`
3. `state_dir` in the config, relative to the config file
4. `.vmlite` next to the config

Every command that operates on an existing VM accepts either a config path or a
managed VM name. A bare name resolves to `<state-dir>/<name>.json`:

```bash
vmlite status dev
vmlite inspect --json dev
vmlite ssh --command 'uname -a' dev
vmlite destroy --force dev
```

When a name is used without `--state-dir` or `VMLITE_STATE_DIR`, vmlite reads
`./vm.json` to discover its configured `state_dir`; otherwise it uses
`./.vmlite`.

Flags belong after the command:

```bash
vmlite up --connect qemu:///system vm.json
```

The shorthand `vmlite --connect qemu:///system vm.json` also works because it
is an implicit `up`.

## Help

```bash
vmlite help
vmlite help up
vmlite up --help
vmlite --help
vmlite version
vmlite --version
```

Command help is rendered from the actual flag definitions, so options and
defaults match the binary. Flags accept either `-name` or `--name`.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Runtime or configuration error |
| `2` | Usage error |
| `3` | VM is not running, missing, or not created (`status`) |

## Commands

### `vmlite up`

Create or start the VM.

```text
vmlite up [--connect URI] [--state-dir DIR] [--recreate] [--dry-run] [--offline] [config.json]
```

| Flag | Meaning |
|---|---|
| `--connect URI` | libvirt connection; defaults to system mode when available |
| `--state-dir DIR` | state directory; defaults to `.vmlite` next to the config |
| `--recreate` | destroy the managed VM and recreate it |
| `--dry-run` | print actions without changing anything |
| `--offline` | do not download missing images |

Behavior:

- an unchanged running VM is a no-op
- a stopped VM is started
- config or connection drift requires `--recreate`
- automated ISO installers start the installer and return
- `cloud_image` and prebuilt `disk` sources may auto-provision

Examples:

```bash
vmlite up
vmlite up vm.json
vmlite up --connect qemu:///system vm.json
vmlite up --dry-run vm.json
vmlite up --recreate vm.json
```

### `vmlite init`

Write a starter configuration from an embedded example template.

```text
vmlite init [--template NAME] [--name NAME] [--force] [--stdout] [--list] [config.json]
```

| Flag | Meaning |
|---|---|
| `--template NAME` | example template; defaults to `cloud-image` |
| `--name NAME` | override the template’s VM name |
| `--force` | overwrite an existing config |
| `--stdout` | print the config instead of writing it |
| `--list` | list available templates |

The default output is `vm.json`. Templates live in `examples/` and include
cloud images, automated Linux installers, Windows, manual ISO, prebuilt disk,
provisioner uploads, and a `minimal` runnable VM.

The `cloud-image` template uses a pinned URL and digest; the first `up`
downloads it into the cache. Installer and ISO templates reference local media
paths that must be edited before booting.

Examples:

```bash
vmlite init
vmlite init --template minimal
vmlite init --template arch-script --name arch-dev
vmlite init --list
vmlite init --stdout --template cloud-image
```

Use `vmlite validate vm.json` after editing installer or ISO media paths.

### `vmlite provision`

Run pending provisioners, or finish an automated installer.

```text
vmlite provision [--state-dir DIR] [--force] [--dry-run] [--timeout DURATION] [name|config.json]
```

| Flag | Meaning |
|---|---|
| `--state-dir DIR` | state directory containing the VM state |
| `--force` | rerun `once` provisioners |
| `--dry-run` | print actions without changing the guest |
| `--timeout DURATION` | maximum wait for guest IP/SSH readiness |

Defaults:

- cloud image / disk / manual ISO: 5 minutes
- Linux installers: 10 minutes for SSH after the install completes
- Windows SSH-completing installers: 30 minutes

If an automated installer is still running, `provision` waits for it to expose
SSH. For Linux/BSD installers it waits for the installer domain to shut off,
ejects the media, and boots the installed disk.

Examples:

```bash
vmlite provision vm.json
vmlite provision dev
vmlite provision --force dev
vmlite provision --timeout 45m dev
```

### `vmlite status`

Show VM, installer, IP, and provisioner state.

```text
vmlite status [--state-dir DIR] [--json] [name|config.json]
```

| Flag | Meaning |
|---|---|
| `--state-dir DIR` | state directory containing the VM state |
| `--json` | machine-readable output |

Human output:

```text
Name:  dev
URI:   qemu:///system
State: running
Disk:  /project/.vmlite/dev/disk.qcow2
Image: /var/cache/vmlite/ubuntu.img
IP:    192.168.122.42
Provisioned: 2 script(s)
```

JSON output:

```json
{
  "name": "dev",
  "config_path": "/project/vm.json",
  "uri": "qemu:///system",
  "state": "running",
  "running": true,
  "disk": "/project/.vmlite/dev/disk.qcow2",
  "image": "/var/cache/vmlite/ubuntu.img",
  "ip": "192.168.122.42",
  "installer": "ubuntu_autoinstall",
  "installer_completion": "shutdown",
  "install_phase": "installed",
  "provisioned": [
    {
      "id": "inline-0",
      "sha256": "...",
      "completed_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

`status` exits `3` when the VM is not running, missing, or not created. JSON is
still emitted in those cases with `running: false` and a descriptive `state`.

### `vmlite inspect`

Show detailed, read-only VM diagnostics: domain state, UUID, CPU/memory,
disks and backing chains, media, network interfaces, live addresses, installer
state, provisioners, and managed paths.

```text
vmlite inspect [--state-dir DIR] [--json|--xml] [name|config.json]
```

| Flag | Meaning |
|---|---|
| `--json` | structured output for scripts |
| `--xml` | raw libvirt domain XML for bug reports |

Examples:

```bash
vmlite inspect vm.json
vmlite inspect dev
vmlite inspect --json dev
vmlite inspect --xml dev > domain.xml
```

`inspect` is read-only and never changes domain or pool state. It exits `3` when
the VM is missing or has not been created.

### `vmlite start`

Start a stopped VM.

```text
vmlite start [--state-dir DIR] [--dry-run] [name|config.json]
```

Examples:

```bash
vmlite start vm.json
vmlite start dev
```

### `vmlite stop`

Gracefully stop a running VM with ACPI shutdown.

```text
vmlite stop [--state-dir DIR] [--timeout DURATION] [--force] [--dry-run] [name|config.json]
```

`--timeout` defaults to `2m`. If the guest does not stop in time, `stop` fails
unless `--force` is passed; `--force` destroys the domain.

The VM can be selected by managed name, so the config file is not required:

```bash
vmlite stop dev
vmlite stop --force dev
vmlite stop --state-dir /path/to/.vmlite dev
```

### `vmlite restart`

Reboot a running VM, or start a stopped VM.

```text
vmlite restart [--state-dir DIR] [--dry-run] [name|config.json]
```

Examples:

```bash
vmlite restart vm.json
vmlite restart dev
```

### `vmlite ssh`

Open an interactive shell or run one remote command.

```text
vmlite ssh [--state-dir DIR] [--user USER] [--command COMMAND] [--timeout DURATION] [--dry-run] [name|config.json]
```

- Resolves the guest IP from state and waits up to `--timeout` (default `30s`)
  for IP and SSH readiness.
- Uses the managed SSH key and user from state.
- Requires `qemu:///system` with a guest-visible network.
- `--command` runs non-interactively; otherwise an interactive shell is opened.
- `--print-config` prints a `Host` block for `~/.ssh/config` instead of
  connecting.

Examples:

```bash
vmlite ssh vm.json
vmlite ssh dev
vmlite ssh --command 'uname -a' dev
vmlite ssh --user root dev
vmlite ssh --print-config dev
```

`--command` runs through the remote shell. To send a local multi-line script:

```bash
vmlite ssh --command 'bash -s' dev < bootstrap.sh
```

For a daemon, detach its stdio so the SSH command returns:

```bash
vmlite ssh --command 'setsid sh -c "exec ./server >/var/log/server.log 2>&1" </dev/null >/dev/null 2>&1 &' dev
```

### `vmlite scp`

Copy a host file or directory into the guest.

```text
vmlite scp [--state-dir DIR] [--user USER] [--timeout DURATION] [--recursive] [--dry-run] <local> <remote> [name|config.json]
```

Examples:

```bash
vmlite scp ./app.conf /etc/app.conf vm.json
vmlite scp ./app.conf /etc/app.conf dev
vmlite scp --recursive ./assets /opt/assets dev
```

### `vmlite console`

Attach to the serial console or print graphical display details.

```text
vmlite console [--state-dir DIR] [--type auto|serial|vnc] [--log] [--log-file PATH] [--dry-run] [name|config.json]
```

- `auto` prints VNC/SPICE details when a display exists, otherwise attaches to
  the serial console.
- `vnc` only prints the display and a `virt-viewer` hint.
- `serial` attaches with `virsh console --force`.
- `--log` records serial output to `.vmlite/<name>/console.log`.
- `--log-file PATH` records to a custom path and implies `--log`.
- Serial logging uses the host `script` utility and requires `--type serial`
  or a serial `auto` console.

Examples:

```bash
vmlite console vm.json
vmlite console dev
vmlite console --type vnc dev
vmlite console --log dev
```

### `vmlite list`

List managed VMs in a state directory.

```text
vmlite list [--state-dir DIR] [--json]
```

Defaults to `VMLITE_STATE_DIR`, `state_dir` from `./vm.json` when present, or
`.vmlite` in the current directory. Human output includes name, state, IP,
installer, phase, and provisioned count.



Remove the VM and managed files.

```text
vmlite destroy [--state-dir DIR] [--force] [--dry-run] [name|config.json]
```

`destroy` refuses to remove a running VM unless `--force` is passed. It removes
the domain, managed disk, seed/installer media, and project asset directory.
The image cache is kept for fast recreation.

Examples:

```bash
vmlite destroy vm.json
vmlite destroy dev
vmlite destroy --force dev
```

### `vmlite validate`

Validate a configuration without creating or changing a VM.

```text
vmlite validate [config.json]
```

Checks:

- JSON parsing and field validation
- required host tools (`qemu-img`, ISO tools, `xorriso` when remastering)
- local source, extra-disk, mount, and SSH-key paths
- installer answer-file access
- installer-specific spec generation

Example:

```bash
vmlite validate vm.json
# vm.json: valid
```

### `vmlite schema`

Print the JSON Schema for vmlite configuration files.

```bash
vmlite schema > vmlite.schema.json
```

Use it in editors for validation and autocomplete. The same schema is checked
into `schema/vmlite.schema.json`.

### `vmlite install`

Install host dependencies and the `vmlite` binary.

```text
vmlite install [--prefix DIR] [--binary PATH] [--binary-url URL] [--binary-sha256 SHA256] [--no-deps] [--dry-run]
```

`install.sh` is a bootstrap that builds a temporary `vmlite` and delegates to
this command. `--prefix` defaults to `/usr/local/bin`.

If vmlite was installed from a `.deb` or `.rpm`, the package manager owns
`/usr/bin/vmlite`; do not run `vmlite install` on top of it.

### `vmlite doctor`

Check host readiness without changing anything.

```text
vmlite doctor [--prefix DIR]
```

Checks KVM, libvirt, `virt-install`, ISO tools, SSH tools, the default network,
and the installed `vmlite` binary. Without `--prefix`, it searches `PATH`, so
package installs in `/usr/bin` are detected.

### `vmlite version`

Print the vmlite version.

```text
vmlite version
vmlite --version
```

Release builds set the version with:

```bash
go build -ldflags "-X main.version=v0.1.0" -o vmlite .
```

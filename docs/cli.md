# vmlite CLI

`vmlite` creates and manages local KVM/libvirt guests from a JSON configuration
file. State is project-local under `.vmlite/`.

## Synopsis

```text
vmlite [command] [options] [config.json]
```

`config.json` defaults to `vm.json`. Running `vmlite` with no command is the
same as `vmlite up`.

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

### `vmlite provision`

Run pending provisioners, or finish an automated installer.

```text
vmlite provision [--state-dir DIR] [--force] [--dry-run] [--timeout DURATION] [config.json]
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
vmlite provision --force vm.json
vmlite provision --timeout 45m vm.json
```

### `vmlite status`

Show VM, installer, IP, and provisioner state.

```text
vmlite status [--state-dir DIR] [--json] [config.json]
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

### `vmlite start`

Start a stopped VM.

```text
vmlite start [--state-dir DIR] [--dry-run] [config.json]
```

### `vmlite stop`

Gracefully stop a running VM with ACPI shutdown.

```text
vmlite stop [--state-dir DIR] [--timeout DURATION] [--force] [--dry-run] [config.json]
```

`--timeout` defaults to `2m`. If the guest does not stop in time, `stop` fails
unless `--force` is passed; `--force` destroys the domain.

### `vmlite restart`

Reboot a running VM, or start a stopped VM.

```text
vmlite restart [--state-dir DIR] [--dry-run] [config.json]
```

### `vmlite ssh`

Open an interactive shell or run one remote command.

```text
vmlite ssh [--state-dir DIR] [--user USER] [--command COMMAND] [--timeout DURATION] [--dry-run] [config.json]
```

- Resolves the guest IP from state and waits up to `--timeout` (default `30s`)
  for IP and SSH readiness.
- Uses the managed SSH key and user from state.
- Requires `qemu:///system` with a guest-visible network.
- `--command` runs non-interactively; otherwise an interactive shell is opened.

Examples:

```bash
vmlite ssh vm.json
vmlite ssh --command 'uname -a' vm.json
vmlite ssh --user root vm.json
```

### `vmlite scp`

Copy a host file or directory into the guest.

```text
vmlite scp [--state-dir DIR] [--user USER] [--timeout DURATION] [--recursive] [--dry-run] <local> <remote> [config.json]
```

Examples:

```bash
vmlite scp ./app.conf /etc/app.conf vm.json
vmlite scp --recursive ./assets /opt/assets vm.json
```

### `vmlite console`

Attach to the serial console or print graphical display details.

```text
vmlite console [--state-dir DIR] [--type auto|serial|vnc] [--dry-run] [config.json]
```

- `auto` prints VNC/SPICE details when a display exists, otherwise attaches to
  the serial console.
- `vnc` only prints the display and a `virt-viewer` hint.
- `serial` attaches with `virsh console --force`.

### `vmlite list`

List managed VMs in a state directory.

```text
vmlite list [--state-dir DIR] [--json]
```

Defaults to `.vmlite` in the current directory. Human output includes name,
state, IP, installer, phase, and provisioned count.



Remove the VM and managed files.

```text
vmlite destroy [--state-dir DIR] [--force] [--dry-run] [config.json]
```

`destroy` refuses to remove a running VM unless `--force` is passed. It removes
the domain, managed disk, seed/installer media, and project asset directory.
The image cache is kept for fast recreation.

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
this command.

### `vmlite doctor`

Check host readiness without changing anything.

```text
vmlite doctor [--prefix DIR]
```

Checks KVM, libvirt, `virt-install`, ISO tools, SSH tools, the default network,
and the installed `vmlite` binary.

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

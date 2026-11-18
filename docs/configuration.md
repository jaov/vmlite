# Configuration reference

`vmlite` reads a JSON file, by default `vm.json`.

Create a starter file from an example template:

```bash
vmlite init
vmlite init --template minimal
vmlite init --template arch-script --name arch-dev
vmlite init --list
```

The default `cloud-image` template downloads a pinned Ubuntu cloud image on
first `up`, so it validates and runs without editing media paths. The `minimal`
template is a runnable blank VM and needs no media.

The machine-readable schema is available with:

```bash
vmlite schema > vmlite.schema.json
```

Example configurations live in `examples/`.

## Minimal example

```json
{
  "name": "dev",
  "memory": 2048,
  "cpus": 2,
  "source": {
    "type": "cloud_image",
    "url": "https://cloud-images.ubuntu.com/minimal/releases/noble/release-20261017/ubuntu-24.04-minimal-cloudimg-amd64.img",
    "sha256": "b3296484e362ba2e713489a5de6225c719d9500962bde25d15f2f21e512c7658"
  },
  "disk": { "size": 10 },
  "network": { "mode": "auto" },
  "graphics": { "type": "none" }
}
```

## Top-level fields

| Field | Type | Meaning |
|---|---|---|
| `name` | string | VM/domain name; required |
| `state_dir` | string | state directory; relative paths resolve from the config file |
| `autostart` | bool | enable libvirt autostart at host boot |
| `memory` | int | memory in MiB |
| `cpus` | int | vCPU count |
| `os_variant` | string | libvirt/osinfo variant, e.g. `ubuntu24.04`, `win11` |
| `firmware` | string | `bios` or `uefi` |
| `secure_boot` | bool | requires `firmware: "uefi"` |
| `tpm` | object | emulated TPM; requires UEFI |
| `source` | object | boot/install source |
| `disk` | object | primary disk |
| `mounts` | array | host directories exposed to the guest |
| `network` | object | network mode/model |
| `graphics` | object | `none`, `vnc`, or `spice` |
| `autoconsole` | bool | keep the virt-install console attached |
| `extra_disks` | array | additional disks or CD-ROMs |
| `ssh` | object | guest account and key for provisioners |
| `provisioners` | array | post-boot scripts |

### TPM

```json
"tpm": { "version": "2.0", "backend": "emulator" }
```

Supported values:

- `version`: `1.2` or `2.0` (default `2.0`)
- `backend`: `emulator`

TPM and Secure Boot require `firmware: "uefi"`.

## Source

| Type | Required fields | Meaning |
|---|---|---|
| `none` | `disk.size` | blank VM with no OS install |
| `cloud_image` | `path` or `url` + `sha256` | boot a cloud image with an overlay |
| `install_tree` | `url` | virt-install location install |
| `iso` | `path` | installer or live ISO |
| `disk` | `path` | import an already-installed disk |
| `pxe` | `disk.size` | network boot |

Common fields:

```json
"source": {
  "type": "iso",
  "path": "/isos/ubuntu-24.04.iso",
  "installer": { "type": "ubuntu_autoinstall" }
}
```

### URL media and cache

Cloud images, installer ISOs, and imported disks can use `source.url`:

```json
"source": {
  "type": "cloud_image",
  "url": "https://cloud-images.ubuntu.com/minimal/releases/noble/release/ubuntu-24.04-minimal-cloudimg-amd64.img",
  "sha256": "<64-character digest>",
  "cache": "/var/tmp/vmlite-cache",
  "cloud_init": "./cloud-init.yaml"
}
```

The same fields work for installer/media sources:

```json
"source": {
  "type": "iso",
  "url": "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-13.7.0-amd64-netinst.iso",
  "sha256": "<64-character digest>",
  "cache": "/var/tmp/vmlite-cache",
  "installer": { "type": "debian_preseed" }
}
```

```json
"source": {
  "type": "disk",
  "url": "https://example.invalid/ubuntu-postgres.qcow2",
  "sha256": "<64-character digest>",
  "cache": "/var/tmp/vmlite-cache"
}
```

- URL media requires `sha256`.
- Local media with `sha256` is verified on use.
- Cached media is verified on every use.
- Default cache is `~/.cache/vmlite`.
- `cache` is a directory. URL media is stored as
  `<first 16 hex of sha256>-<URL basename>`.
- To reuse a pre-downloaded file, set `source.path` to the file. Alternatively,
  copy it to the generated cache path and use `--offline`.
- There are no cache management subcommands yet; remove files manually.
- `vmlite up --offline` refuses to download missing media.
- `destroy` keeps the cache.

## Disk

```json
"disk": {
  "size": 20,
  "path": "/var/tmp/vm.qcow2",
  "format": "qcow2",
  "bus": "virtio"
}
```

| Field | Values |
|---|---|
| `size` | GiB; required for new disks |
| `path` | optional explicit path |
| `format` | `auto`, `qcow2`, `raw` |
| `bus` | `auto`, `virtio`, `sata`, `scsi` |

Use `sata` for Windows Setup unless virtio drivers are integrated into the
answer media.

## Extra disks

```json
"extra_disks": [
  {
    "path": "/isos/virtio-win.iso",
    "device": "cdrom",
    "format": "raw",
    "readonly": true
  }
]
```

- `device`: `disk` or `cdrom`
- `format`: `auto`, `qcow2`, `raw`
- `readonly`: boolean

## Mounts

```json
"mounts": [
  { "source": "/var/tmp/shared", "target": "shared_data", "type": "9p" }
]
```

- `type`: `9p` (default), `none`, or `virtiofs` (not implemented yet)
- `source` must exist on the host
- 9p requires guest kernel support

## Network

```json
"network": { "mode": "default", "model": "virtio" }
```

| Mode | Meaning |
|---|---|
| `auto` | system default network, or user networking in session mode |
| `default` | libvirt default NAT network |
| `user` | QEMU user networking |
| `bridge` | host bridge; requires `bridge` |

Provisioners require a guest-visible network. Use system mode with `default` or
a bridge, not `user`.

## Graphics

```json
"graphics": { "type": "vnc", "listen": "127.0.0.1" }
```

- `none`
- `vnc`
- `spice`

## SSH and provisioners

```json
"ssh": { "user": "vmlite", "key": "/home/me/.ssh/id_ed25519" },
"provisioners": [
  {
    "type": "shell",
    "path": "./provision/setup.sh",
    "run": "once",
    "uploads": [
      { "source": "./config/app.conf", "target": "/etc/app.conf" }
    ],
    "env": { "APP_ENV": "dev", "RETRIES": "3" }
  },
  { "type": "shell", "inline": "echo hi >> /var/tmp/log", "run": "always" }
]
```

- `ssh.key` is optional for cloud images and automated ISO installers; vmlite
  generates a key when it can deliver it.
- Manual ISO and prebuilt disk sources require `ssh.key`.
- `ssh.user` defaults to `vmlite`, or `Administrator` for Windows.
- `run`: `once` (default) or `always`.
- `type`: `shell` (default) or `powershell`.
- `uploads` copies host files into the guest before the script. Sources are
  relative to the config file; directories are not supported yet.
- `env` is passed through the remote environment for shell provisioners and
  emitted as `$env:NAME = 'value'` lines for PowerShell.
- The `once` hash includes script contents, environment values, and uploaded
  file contents.

### Provisioner lifecycle

- A new `cloud_image` or `disk` VM runs provisioners during its first `up`.
- Later `up` calls do not rerun provisioners; use `vmlite provision`.
- Installer sources run provisioners through `vmlite provision` after the
  installer finishes.
- `run: once` runs when its hash changes. `vmlite provision --force` reruns it
  even when the hash matches.
- `run: always` runs on every `vmlite provision` and during first-creation
  provisioning.
- Uploads are applied only when the provisioner runs. A skipped `once`
  provisioner does not re-upload its files.
- Script stdout and stderr are streamed to the terminal.
- `--timeout` controls IP/SSH readiness, not script runtime. There is no
  per-provisioner timeout.

`shell` uses `bash -s` with `sudo -n` for non-root users. `powershell` uses
`powershell -NoProfile -NonInteractive -Command -` without sudo.

## Installer

```json
"source": {
  "type": "iso",
  "path": "/isos/image.iso",
  "installer": {
    "type": "ubuntu_autoinstall",
    "config": "./answer-file"
  }
}
```

| Field | Meaning |
|---|---|
| `type` | installer provider |
| `config` | answer file path, relative to the config file |
| `inline` | answer file contents |
| `windows` | generated Windows autounattend options |

Installer types:

| Type | Media |
|---|---|
| `ubuntu_autoinstall` | Ubuntu live-server ISO |
| `fedora_kickstart` | Fedora Server ISO |
| `debian_preseed` | Debian netinst ISO |
| `arch_script` | Arch Linux ISO |
| `windows_autounattend` | Windows Server/client ISO |

See `docs/installers.md` for per-installer details.

### Windows options

```json
"installer": {
  "type": "windows_autounattend",
  "windows": {
    "locale": "en-US",
    "input_locale": "0409:00000409",
    "timezone": "UTC",
    "image_index": 1,
    "admin_password": "..."
  }
}
```

If `admin_password` is omitted, vmlite generates one and writes it to
`.vmlite/<name>/windows-admin-password` with mode `0600`.

## State

Managed files live in `.vmlite/` next to the config by default. The location is
selected in this order:

1. `--state-dir`
2. `VMLITE_STATE_DIR`
3. `state_dir` in the config, relative to the config file
4. `.vmlite` next to the config

```json
"state_dir": "/var/lib/vmlite/dev"
```

```text
.vmlite/
  <name>.json
  <name>/
    disk.qcow2
    id_ed25519
    id_ed25519.pub
    *-seed.iso
    *-installer.iso
    windows-admin-password
```

`vmlite destroy` removes the managed assets; the image cache is kept.

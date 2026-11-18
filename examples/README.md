# Example configurations

| File | Purpose |
|---|---|
| `minimal.json` | runnable blank VM with no OS media |
| `cloud-image.json` | Ubuntu cloud image |
| `ubuntu-autoinstall.json` | Ubuntu autoinstall ISO |
| `fedora-kickstart.json` | Fedora kickstart ISO |
| `debian-preseed.json` | Debian preseed ISO |
| `arch-script.json` | Arch archiso scripted install |
| `windows-autounattend.json` | Windows 11 autounattend |
| `manual-iso.json` | manual OpenBSD install + user SSH key |
| `prebuilt-disk.json` | imported installed disk |
| `provisioner-uploads.json` | provisioner uploads and env |
| `winpe.json` | WinPE CI marker disk |

Create a starter config from a template:

```bash
vmlite init
vmlite init --template minimal
vmlite init --list
```

Validate an example:

```bash
vmlite validate examples/cloud-image.json
```

Most examples reference media under `/isos`, `/images`, or `/var/cache/vmlite`;
replace those paths with local media.

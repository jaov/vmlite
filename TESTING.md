# Testing vmlite

## Unit tests

Unit tests need no libvirt and are safe to run anywhere:

```bash
go test ./...
```

They cover config parsing/validation, argument construction, URI-independent
preflight logic (using fake `virsh`/`virt-install` scripts), and CLI usage
errors. Tests that require `/dev/kvm` skip automatically.

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
| `VMLITE_TEST_SYSTEM=1` | also run the system-mode test against `qemu:///system` |
| `VMLITE_TEST_KEEP=1` | keep domains and disks after the test for debugging |

Run both session and system mode:

```bash
VMLITE_TEST_SYSTEM=1 go test -tags integration -v ./...
```

### Inspecting leftovers

Cleanup normally leaves nothing behind. To check:

```bash
virsh -c qemu:///session list --all
virsh -c qemu:///system list --all
```

Any leftover domains are named `vmlite-it-<pid>-<n>` and can be removed with:

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

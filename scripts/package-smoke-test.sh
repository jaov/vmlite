#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: package-smoke-test.sh debian|fedora PACKAGE

Install a vmlite .deb or .rpm in a fresh cloud-image VM using a provisioner,
verify the installed files and CLI, then uninstall it.

Required environment:
  VMLITE_TEST_DEBIAN_IMAGE   Debian generic cloud qcow2 (for debian)
  VMLITE_TEST_FEDORA_IMAGE   Fedora Cloud Base qcow2 (for fedora)

Optional:
  VMLITE_BIN                 vmlite binary to use (default: vmlite from PATH)
  VMLITE_CONNECT             libvirt URI (default: qemu:///system)
  VMLITE_TEST_KEEP=1         keep the VM and work directory
  VMLITE_WORKDIR             work directory (default: mktemp under /tmp)

Example:
  VMLITE_TEST_DEBIAN_IMAGE=/tmp/debian-12-genericcloud-amd64.qcow2 \
    scripts/package-smoke-test.sh debian dist/vmlite_0.1.0-1_amd64.deb
EOF
}

if [[ $# -ne 2 ]]; then
  usage
  exit 2
fi

os="$1"
package="$2"
case "$os" in
  debian|fedora) ;;
  *)
    echo "unsupported OS: $os" >&2
    usage
    exit 2
    ;;
esac

if [[ ! -f "$package" ]]; then
  echo "package not found: $package" >&2
  exit 1
fi
package="$(readlink -f "$package")"

vmlite="${VMLITE_BIN:-vmlite}"
if ! command -v "$vmlite" >/dev/null 2>&1; then
  echo "vmlite binary not found: $vmlite (set VMLITE_BIN)" >&2
  exit 1
fi
uri="${VMLITE_CONNECT:-qemu:///system}"

case "$os" in
  debian)
    image="${VMLITE_TEST_DEBIAN_IMAGE:-}"
    variant="debian12"
    remote_pkg="/tmp/vmlite-package.deb"
    install_cmd='sudo apt-get -qq update && sudo DEBIAN_FRONTEND=noninteractive apt-get -qq install -y $VMLITE_PACKAGE'
    remove_cmd='sudo DEBIAN_FRONTEND=noninteractive apt-get -qq remove -y vmlite'
    ;;
  fedora)
    image="${VMLITE_TEST_FEDORA_IMAGE:-}"
    variant="fedora42"
    remote_pkg="/tmp/vmlite-package.rpm"
    install_cmd='sudo dnf -q install -y --nogpgcheck $VMLITE_PACKAGE'
    remove_cmd='sudo dnf -q remove -y vmlite'
    ;;
esac

if [[ -z "$image" ]]; then
  echo "set VMLITE_TEST_$(tr '[:lower:]' '[:upper:]' <<<"$os")_IMAGE to a cloud image" >&2
  exit 1
fi
if [[ ! -f "$image" ]]; then
  echo "cloud image not found: $image" >&2
  exit 1
fi

work="${VMLITE_WORKDIR:-}"
if [[ -z "$work" ]]; then
  work="$(mktemp -d /tmp/vmlite-pkgtest-XXXXXX)"
fi
mkdir -p "$work"
chmod 0755 "$work"
name="vmlite-pkgtest-$os-$$"
config="$work/vm.json"
state="$work/.vmlite"
pool="$(basename "$work")"
up_log="$work/up.log"
status_log="$work/status.log"
skip_log="$work/provision-skip.log"

cleanup() {
  local rc=$?
  trap - EXIT
  if [[ "${VMLITE_TEST_KEEP:-0}" == "1" ]]; then
    echo "VMLITE_TEST_KEEP=1: keeping $work" >&2
    exit "$rc"
  fi
  "$vmlite" destroy --force --state-dir "$state" "$config" >/dev/null 2>&1 || true
  virsh -c "$uri" pool-destroy "$pool" >/dev/null 2>&1 || true
  virsh -c "$uri" pool-undefine "$pool" >/dev/null 2>&1 || true
  rm -rf "$work"
  exit "$rc"
}
trap cleanup EXIT

# The provisioner runs through sudo, so the env wrapper must stay after
# sudo -n. VMLITE_PACKAGE is provided by the provisioner env block.
inline="set -e; test -s \$VMLITE_PACKAGE; echo package=\$VMLITE_PACKAGE; $install_cmd; command -v vmlite; vmlite version; vmlite --help >/dev/null; test -f /usr/share/man/man1/vmlite.1; vmlite doctor >/tmp/vmlite-doctor.txt 2>&1 || true; cat /tmp/vmlite-doctor.txt; grep -E 'vmlite: /usr/(s?bin)/vmlite' /tmp/vmlite-doctor.txt; $remove_cmd; hash -r; ! command -v vmlite >/dev/null 2>&1; test ! -e /usr/bin/vmlite; test ! -e /usr/share/man/man1/vmlite.1; echo PACKAGE_SMOKE_OK"

cat >"$config" <<EOF
{
  "name": "$name",
  "memory": 2048,
  "cpus": 2,
  "firmware": "bios",
  "os_variant": "$variant",
  "source": {
    "type": "cloud_image",
    "path": "$image"
  },
  "disk": { "size": 8, "path": "$work/disk.qcow2", "format": "qcow2" },
  "network": { "mode": "default" },
  "graphics": { "type": "none" },
  "ssh": { "user": "vmlite" },
  "provisioners": [
    {
      "type": "shell",
      "run": "once",
      "uploads": [
        { "source": "$package", "target": "$remote_pkg" }
      ],
      "env": {
        "VMLITE_PACKAGE": "$remote_pkg",
        "VMLITE_PACKAGE_KIND": "$os"
      },
      "inline": "$inline"
    }
  ]
}
EOF

echo "Creating $os VM and installing $package through a provisioner"
"$vmlite" up --connect "$uri" --state-dir "$state" "$config" 2>&1 | tee "$up_log"
grep -F 'PACKAGE_SMOKE_OK' "$up_log" >/dev/null

"$vmlite" status --state-dir "$state" "$config" 2>&1 | tee "$status_log"
grep -F 'Provisioned: 1 script(s)' "$status_log" >/dev/null

"$vmlite" provision --state-dir "$state" "$config" 2>&1 | tee "$skip_log"
grep -F 'already satisfied' "$skip_log" >/dev/null

echo "Package smoke test passed for $os"

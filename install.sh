#!/usr/bin/env bash
set -euo pipefail

PREFIX="/usr/local/bin"
BINARY=""
DO_DEPS=1
DRY_RUN=0

usage() {
	cat <<'EOF'
Usage: ./install.sh [options]

Install vmlite and its host dependencies.

Options:
  --binary PATH   Install a prebuilt vmlite binary instead of building one
  --prefix DIR    Install destination (default: /usr/local/bin)
  --no-deps       Skip host package installation
  --dry-run       Print commands without changing the host
  -h, --help      Show this help
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--binary)
			BINARY="${2:-}"
			shift 2
			;;
		--prefix)
			PREFIX="${2:-}"
			shift 2
			;;
		--no-deps)
			DO_DEPS=0
			shift
			;;
		--dry-run)
			DRY_RUN=1
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "unknown option: $1" >&2
			usage >&2
			exit 2
			;;
	esac
done

if [ -z "$BINARY" ] && [ -f ./vmlite ] && [ -x ./vmlite ]; then
	BINARY="./vmlite"
fi

run() {
	if [ "$DRY_RUN" -eq 1 ]; then
		printf '+'
		printf ' %q' "$@"
		printf '\n'
	else
		"$@"
	fi
}

die() {
	echo "install.sh: $*" >&2
	exit 1
}

if [ ! -r /etc/os-release ]; then
	die "cannot detect the Linux distribution (/etc/os-release is missing)"
fi
# shellcheck disable=SC1091
. /etc/os-release

DISTRO_ID="${ID:-unknown}"
DISTRO_LIKE="${ID_LIKE:-}"
FAMILY=""

case " $DISTRO_ID $DISTRO_LIKE " in
	*" debian "*|*" ubuntu "*)
		FAMILY="debian"
		;;
	*" fedora "*|*" rhel "*|*" centos "*|*" rocky "*|*" alma "*)
		FAMILY="fedora"
		;;
	*" arch "*)
		FAMILY="arch"
		;;
	*" opensuse "*|*" suse "*|*" sles "*)
		FAMILY="suse"
		;;
	*" freebsd "*)
		die "FreeBSD is not supported as a vmlite host: vmlite requires Linux KVM, libvirt, and virt-install"
		;;
	*)
		die "unsupported distribution: ID=$DISTRO_ID ID_LIKE=$DISTRO_LIKE"
		;;
esac

if [ "$(id -u)" -eq 0 ]; then
	SUDO=()
else
	command -v sudo >/dev/null 2>&1 || die "root or sudo is required to install packages"
	SUDO=(sudo)
fi

case "$FAMILY" in
	debian)
		PKGS=(qemu-system-x86 qemu-utils libvirt-daemon-system libvirt-clients virtinst xorriso)
		;;
	fedora)
		PKGS=(qemu-kvm qemu-img libvirt libvirt-daemon-kvm libvirt-daemon-config-network libvirt-client virt-install xorriso)
		;;
	arch)
		PKGS=(qemu-full libvirt virt-install xorriso dnsmasq)
		;;
	suse)
		PKGS=(qemu-kvm libvirt-daemon-qemu libvirt-client virt-install xorriso)
		;;
esac

if [ "$DO_DEPS" -eq 1 ]; then
	case "$FAMILY" in
		debian)
			run "${SUDO[@]}" apt-get update
			run "${SUDO[@]}" env DEBIAN_FRONTEND=noninteractive apt-get install -y "${PKGS[@]}"
			;;
		fedora)
			run "${SUDO[@]}" dnf install -y "${PKGS[@]}"
			;;
		arch)
			run "${SUDO[@]}" pacman -Sy --needed --noconfirm "${PKGS[@]}"
			;;
		suse)
			run "${SUDO[@]}" zypper --non-interactive install "${PKGS[@]}"
			;;
	esac
fi

if [ -z "$BINARY" ]; then
	command -v go >/dev/null 2>&1 || die "Go is not installed; pass --binary PATH or install Go"
	if [ "$DRY_RUN" -eq 1 ]; then
		BINARY="/tmp/vmlite-build"
	else
		BINARY="$(mktemp /tmp/vmlite-build.XXXXXX)"
	fi
	run env CGO_ENABLED=0 go build -o "$BINARY" .
fi

[ "$DRY_RUN" -eq 1 ] || [ -f "$BINARY" ] || die "binary not found: $BINARY"

run "${SUDO[@]}" install -d "$PREFIX"
run "${SUDO[@]}" install -m 0755 "$BINARY" "$PREFIX/vmlite"

if [ "$DRY_RUN" -eq 1 ]; then
	echo "dry run complete for $DISTRO_ID ($FAMILY)"
	exit 0
fi

# KVM can be unavailable in containers, WSL, or when virtualization is off.
modprobe kvm_intel >/dev/null 2>&1 || modprobe kvm_amd >/dev/null 2>&1 || true
if [ ! -e /dev/kvm ]; then
	echo "install.sh: warning: /dev/kvm is missing; vmlite needs KVM (enable VT-x/AMD-V and load kvm_intel/kvm_amd)" >&2
fi

for tool in virt-install virsh qemu-img; do
	command -v "$tool" >/dev/null 2>&1 || die "required tool is missing after installation: $tool"
done

# Some distributions install the default network but leave it stopped.
if command -v virsh >/dev/null 2>&1; then
	if virsh net-info default >/dev/null 2>&1; then
		virsh net-start default >/dev/null 2>&1 || true
		virsh net-autostart default >/dev/null 2>&1 || true
	fi
fi

# Package managers create these groups; the current user needs them for the
# system libvirt socket after the next login.
if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ]; then
	if getent group libvirt >/dev/null 2>&1; then
		usermod -aG libvirt "$SUDO_USER" || true
	fi
	if getent group kvm >/dev/null 2>&1; then
		usermod -aG kvm "$SUDO_USER" || true
	fi
	echo "install.sh: added $SUDO_USER to libvirt/kvm groups if present; log out and back in"
fi

echo "install.sh: vmlite installed at $PREFIX/vmlite"
"$PREFIX/vmlite" --help >/dev/null 2>&1 || true

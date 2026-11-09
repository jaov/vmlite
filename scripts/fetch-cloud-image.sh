#!/usr/bin/env bash
set -euo pipefail

url="https://cloud-images.ubuntu.com/minimal/releases/noble/release/ubuntu-24.04-minimal-cloudimg-amd64.img"
dest="${1:-${XDG_CACHE_HOME:-$HOME/.cache}/vmlite/ubuntu-24.04-minimal-cloudimg-amd64.img}"

mkdir -p "$(dirname "$dest")"

if [[ -f "$dest" ]]; then
  echo "$dest"
  exit 0
fi

echo "Downloading Ubuntu 24.04 minimal cloud image to $dest" >&2
curl -fL --retry 3 --continue-at - -o "$dest" "$url"
echo "$dest"

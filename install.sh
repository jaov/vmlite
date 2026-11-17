#!/bin/sh
set -eu

# Build relative to the repository, not the caller's working directory.
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

if command -v go >/dev/null 2>&1; then
	tmp="$(mktemp)"
	trap 'rm -f "$tmp"' EXIT HUP INT TERM
	CGO_ENABLED=0 go build -o "$tmp" .
	"$tmp" install "$@"
	exit $?
fi

if [ -x ./vmlite ]; then
	exec ./vmlite install "$@"
fi

echo "install.sh: Go or a prebuilt ./vmlite is required" >&2
exit 1

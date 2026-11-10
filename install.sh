#!/bin/sh
set -eu

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

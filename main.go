package main

import (
	"os"

	"vmlite/internal/cli"
)

// version is overridden with -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	cli.Version = version
	os.Exit(cli.Main(os.Args[1:]))
}

package main

import (
	_ "embed"
	"fmt"
)

//go:embed schema/vmlite.schema.json
var vmliteSchema string

func runSchema(args []string) error {
	if len(args) > 0 {
		return errUsage
	}
	fmt.Print(vmliteSchema)
	return nil
}

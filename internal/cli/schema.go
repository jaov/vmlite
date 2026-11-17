package cli

import (
	"fmt"

	"vmlite/schema"
)

func runSchema(args []string) error {
	if len(args) > 0 {
		return errUsage
	}
	fmt.Print(schema.Text)
	return nil
}

// Package schema embeds the vmlite JSON Schema.
package schema

import _ "embed"

//go:embed vmlite.schema.json
var Text string

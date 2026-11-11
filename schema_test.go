package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSchemaOutput(t *testing.T) {
	out, err := captureCommandOutput(t, func() error { return runSchema(nil) })
	if err != nil {
		t.Fatalf("runSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(out), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if schema["$schema"] == nil || schema["properties"] == nil {
		t.Fatalf("schema missing expected keys: %v", schema)
	}
	for _, want := range []string{"windows_autounattend", "arch_script", "provisioners"} {
		if !strings.Contains(out, want) {
			t.Fatalf("schema missing %q", want)
		}
	}
}

func TestSchemaRejectsArgs(t *testing.T) {
	if err := runSchema([]string{"extra"}); err != errUsage {
		t.Fatalf("runSchema = %v, want errUsage", err)
	}
}

package examples

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestNames(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	for _, want := range []string{"cloud-image", "minimal", "arch-script"} {
		if !slices.Contains(names, want) {
			t.Errorf("Names missing %q: %v", want, names)
		}
	}
}

func TestRead(t *testing.T) {
	data, err := Read("minimal")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("Read returned invalid JSON: %v", err)
	}
	if value["name"] != "minimal" {
		t.Fatalf("name = %v", value["name"])
	}
}

func TestReadRejectsUnknown(t *testing.T) {
	if _, err := Read("missing"); err == nil {
		t.Fatal("Read succeeded for a missing template")
	}
	if _, err := Read("../vm"); err == nil {
		t.Fatal("Read accepted a traversal path")
	}
}

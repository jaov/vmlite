// Package examples exposes the checked-in example configurations.
package examples

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed *.json
var files embed.FS

// Names returns the available template names without the .json suffix.
func Names() ([]string, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read example templates: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".json"))
	}
	sort.Strings(names)
	return names, nil
}

// Read returns the JSON contents of the named template.
func Read(name string) ([]byte, error) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".json")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid template %q", name)
	}
	data, err := files.ReadFile(name + ".json")
	if err != nil {
		return nil, fmt.Errorf("unknown template %q", name)
	}
	return data, nil
}

package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"vmlite/examples"
)

func initFlagSet() (*flag.FlagSet, *string, *string, *bool, *bool, *bool) {
	fs := newStateFlagSet("init")
	template := fs.String("template", "cloud-image", "example template to use")
	name := fs.String("name", "", "override the VM name in the template")
	force := fs.Bool("force", false, "overwrite an existing configuration")
	stdout := fs.Bool("stdout", false, "print the configuration instead of writing a file")
	list := fs.Bool("list", false, "list available templates")
	return fs, template, name, force, stdout, list
}

func runInit(args []string) error {
	fs, template, name, force, stdout, list := initFlagSet()
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return errUsage
	}

	if *list {
		names, err := examples.Names()
		if err != nil {
			return err
		}
		fmt.Println("Available templates:")
		for _, name := range names {
			fmt.Printf("  %s\n", name)
		}
		return nil
	}

	data, err := examples.Read(*template)
	if err != nil {
		return err
	}
	if *name != "" {
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("parse template %q: %w", *template, err)
		}
		value["name"] = *name
		data, err = json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Errorf("encode template %q: %w", *template, err)
		}
		data = append(data, '\n')
	}

	if *stdout {
		_, err := os.Stdout.Write(data)
		return err
	}

	path := "vm.json"
	if len(rest) == 1 {
		path = rest[0]
	}
	if _, err := os.Stat(path); err == nil {
		if !*force {
			return fmt.Errorf("%s already exists; use --force to overwrite", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	fmt.Printf("Created %s from template %s.\n", path, *template)
	fmt.Println("Review source.path/source.url and other settings, then run:")
	fmt.Printf("  vmlite validate %s\n", path)
	fmt.Printf("  vmlite up %s\n", path)
	return nil
}

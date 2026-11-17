package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestTopLevelHelp(t *testing.T) {
	var out bytes.Buffer
	writeTopHelp(&out)
	help := out.String()
	for _, want := range []string{
		"vmlite - reproducible local KVM guests",
		"up", "provision", "status", "inspect", "destroy", "validate", "install", "doctor", "help",
		"vmlite help <command>",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("top-level help missing %q:\n%s", want, help)
		}
	}
}

func TestCommandHelpUsesRealFlags(t *testing.T) {
	cmd, ok := lookupCommand("up")
	if !ok {
		t.Fatal("up command not found")
	}
	var out bytes.Buffer
	writeCommandHelp(&out, cmd)
	help := out.String()
	for _, want := range []string{
		"vmlite up", "Usage:", "Options:",
		"-connect", "-state-dir", "-recreate", "-dry-run", "-offline",
		"Examples:", "vmlite up vm.json",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("up help missing %q:\n%s", want, help)
		}
	}
}

func TestAllCommandHelp(t *testing.T) {
	for _, cmd := range commands() {
		var out bytes.Buffer
		writeCommandHelp(&out, cmd)
		if !strings.Contains(out.String(), cmd.Usage) {
			t.Errorf("%s help missing usage %q:\n%s", cmd.Name, cmd.Usage, out.String())
		}
	}
}

func TestHelpUnknownCommand(t *testing.T) {
	if err := runHelp([]string{"not-a-command"}); !errors.Is(err, errUsage) {
		t.Fatalf("runHelp = %v, want errUsage", err)
	}
}

func TestHelpFlagDoesNotLoadConfig(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := dispatch([]string{"up", "--help"})
	w.Close()
	os.Stdout = old

	data, err := readAll(r)
	if err != nil {
		t.Fatalf("read help: %v", err)
	}
	if runErr != nil {
		t.Fatalf("dispatch help = %v", runErr)
	}
	if !strings.Contains(string(data), "vmlite up - Create or start the VM") {
		t.Fatalf("unexpected help output:\n%s", data)
	}
}

func readAll(r *os.File) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}

package cli

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
		"up", "init", "provision", "status", "inspect", "destroy", "validate", "install", "doctor", "help",
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

func TestNoCommandShowsHelp(t *testing.T) {
	out, err := captureCommandOutput(t, func() error {
		return dispatch(nil)
	})
	if err != nil {
		t.Fatalf("dispatch with no command: %v", err)
	}
	for _, want := range []string{"vmlite - reproducible local KVM guests", "Usage:", "Commands:", "vmlite help <command>"} {
		if !strings.Contains(out, want) {
			t.Errorf("no-command help missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Using libvirt connection") {
		t.Fatalf("no-command dispatch started a VM:\n%s", out)
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

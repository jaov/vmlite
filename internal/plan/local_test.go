package plan

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalRunnerOutput(t *testing.T) {
	r := &LocalRunner{}
	out, err := r.Output(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", `printf hello`},
	})
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if out != "hello" {
		t.Fatalf("output = %q, want hello", out)
	}
}

func TestLocalRunnerOutputEnv(t *testing.T) {
	r := &LocalRunner{}
	out, err := r.Output(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", `printf %s "$FOO"`},
		Env:  []string{"FOO=bar"},
	})
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if out != "bar" {
		t.Fatalf("output = %q, want bar", out)
	}
}

func TestLocalRunnerRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	r := &LocalRunner{}
	err := r.Run(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", `printf hello > "$1"`, "sh", path},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("file = %q, want hello", data)
	}
}

func TestLocalRunnerDryRunSudo(t *testing.T) {
	var out bytes.Buffer
	r := &LocalRunner{DryRun: true, Stdout: &out, Geteuid: func() int { return 1000 }}
	err := r.Run(context.Background(), Command{
		Name: "apt-get",
		Args: []string{"update"},
		Sudo: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "+ sudo apt-get update") {
		t.Fatalf("dry run output = %q", out.String())
	}
}

func TestLocalRunnerDryRunSudoEnv(t *testing.T) {
	var out bytes.Buffer
	r := &LocalRunner{DryRun: true, Stdout: &out, Geteuid: func() int { return 1000 }}
	err := r.Run(context.Background(), Command{
		Name: "apt-get",
		Args: []string{"install", "-y", "qemu"},
		Env:  []string{"DEBIAN_FRONTEND=noninteractive"},
		Sudo: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "+ sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y qemu") {
		t.Fatalf("dry run output = %q", out.String())
	}
}

func TestLocalRunnerDryRunRootHasNoSudo(t *testing.T) {
	var out bytes.Buffer
	r := &LocalRunner{DryRun: true, Stdout: &out, Geteuid: func() int { return 0 }}
	err := r.Run(context.Background(), Command{
		Name: "apt-get",
		Args: []string{"update"},
		Sudo: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(out.String(), "sudo") {
		t.Fatalf("root dry run should not use sudo: %q", out.String())
	}
}

func TestLocalRunnerOutputError(t *testing.T) {
	r := &LocalRunner{}
	_, err := r.Output(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", `echo bad >&2; exit 3`},
	})
	if err == nil {
		t.Fatal("Output succeeded, want error")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error = %q, want stderr detail", err)
	}
}

func TestCommandStringQuotesSpaces(t *testing.T) {
	cmd := Command{Name: "echo", Args: []string{"hello world"}}
	if got := cmd.String(); got != `echo "hello world"` {
		t.Fatalf("String = %q", got)
	}
}

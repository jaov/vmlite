package plan

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeSSH(t *testing.T) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(binDir, "ssh.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SSH_TEST_LOG\"\n/bin/cat >/dev/null\nprintf remote-output\n"
	if err := os.WriteFile(filepath.Join(binDir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	return binDir, logPath
}

func TestSSHRunnerOutput(t *testing.T) {
	binDir, logPath := fakeSSH(t)
	t.Setenv("PATH", binDir)
	t.Setenv("SSH_TEST_LOG", logPath)

	r := &SSHRunner{Host: "192.0.2.10", User: "vmlite", Key: "/tmp/key"}
	out, err := r.Output(context.Background(), Command{
		Name:  "bash",
		Args:  []string{"-s"},
		Sudo:  true,
		Stdin: strings.NewReader("echo hi"),
	})
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if out != "remote-output" {
		t.Fatalf("output = %q", out)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read ssh log: %v", err)
	}
	log := string(data)
	for _, want := range []string{"-i\n/tmp/key", "vmlite@192.0.2.10", "sudo -n bash -s"} {
		if !strings.Contains(log, want) {
			t.Errorf("ssh args missing %q:\n%s", want, log)
		}
	}
}

func TestSSHRunnerDryRun(t *testing.T) {
	var out bytes.Buffer
	r := &SSHRunner{Host: "192.0.2.10", User: "vmlite", Key: "/tmp/key", DryRun: true, Stdout: &out}
	if err := r.Run(context.Background(), Command{Name: "bash", Args: []string{"-s"}, Sudo: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "vmlite@192.0.2.10 sudo -n bash -s") {
		t.Fatalf("dry run = %q", out.String())
	}
}

func TestRemoteCommandQuoting(t *testing.T) {
	got := remoteCommand(Command{Name: "sh", Args: []string{"-c", "echo hello world"}, Sudo: true})
	want := `sudo -n sh -c 'echo hello world'`
	if got != want {
		t.Fatalf("remoteCommand = %q, want %q", got, want)
	}
}

func TestRemoteCommandEnvAfterSudo(t *testing.T) {
	got := remoteCommand(Command{
		Name: "bash",
		Args: []string{"-s"},
		Env:  []string{"APP_ENV=dev", "RETRIES=3"},
		Sudo: true,
	})
	want := `sudo -n env APP_ENV=dev RETRIES=3 bash -s`
	if got != want {
		t.Fatalf("remoteCommand = %q, want %q", got, want)
	}
}

func TestRemoteCommandEnvWithoutSudo(t *testing.T) {
	got := remoteCommand(Command{
		Name: "bash",
		Args: []string{"-s"},
		Env:  []string{"APP_ENV=dev"},
	})
	want := `env APP_ENV=dev bash -s`
	if got != want {
		t.Fatalf("remoteCommand = %q, want %q", got, want)
	}
}

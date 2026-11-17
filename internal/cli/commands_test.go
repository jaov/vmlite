package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func commandFixture(t *testing.T) (configPath, stateDir, stateFile string) {
	t.Helper()
	dir := t.TempDir()
	configPath = filepath.Join(dir, "vm.json")
	if err := os.WriteFile(configPath, []byte(`{
		"name": "cmd-test",
		"memory": 256,
		"cpus": 1,
		"source": {"type": "none"},
		"disk": {"size": 1}
	}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	stateDir = filepath.Join(dir, ".vmlite")
	stateFile = filepath.Join(stateDir, "cmd-test.json")
	st := &State{
		Version:      1,
		Name:         "cmd-test",
		ConfigPath:   configPath,
		LibvirtURI:   systemURI,
		IP:           "192.0.2.10",
		SSHKeyPath:   "/tmp/key",
		SSHUser:      "root",
		InstallPhase: installPhaseInstalled,
	}
	if err := saveState(stateFile, st); err != nil {
		t.Fatalf("save state: %v", err)
	}
	return configPath, stateDir, stateFile
}

func fakeVirsh(t *testing.T, script string) string {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake virsh: %v", err)
	}
	t.Setenv("PATH", binDir)
	return binDir
}

func captureCommandOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = old
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return string(data), runErr
}

func TestStartStopRestart(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		configPath, _, _ := commandFixture(t)
		logPath := filepath.Join(t.TempDir(), "virsh.log")
		fakeVirsh(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+logPath+"\ncase \"$*\" in *' domstate '*) echo 'shut off';; *' start '*) echo started;; esac\nexit 0\n")
		if err := runStart([]string{configPath}); err != nil {
			t.Fatalf("runStart: %v", err)
		}
		data, _ := os.ReadFile(logPath)
		if !strings.Contains(string(data), "start\ncmd-test") {
			t.Fatalf("virsh start not called:\n%s", data)
		}
	})

	t.Run("stop", func(t *testing.T) {
		configPath, _, _ := commandFixture(t)
		logPath := filepath.Join(t.TempDir(), "virsh.log")
		stopFlag := filepath.Join(t.TempDir(), "stopped")
		t.Setenv("STOP_FLAG", stopFlag)
		fakeVirsh(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+logPath+"\ncase \"$*\" in *' domstate '*) if [ -f \"$STOP_FLAG\" ]; then echo 'shut off'; else echo running; fi;; *' shutdown '*) : > \"$STOP_FLAG\"; echo shutdown;; esac\nexit 0\n")
		if err := runStop([]string{"--timeout", "2s", configPath}); err != nil {
			t.Fatalf("runStop: %v", err)
		}
		data, _ := os.ReadFile(logPath)
		if !strings.Contains(string(data), "shutdown\ncmd-test") {
			t.Fatalf("virsh shutdown not called:\n%s", data)
		}
	})

	t.Run("restart", func(t *testing.T) {
		configPath, _, _ := commandFixture(t)
		logPath := filepath.Join(t.TempDir(), "virsh.log")
		fakeVirsh(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+logPath+"\ncase \"$*\" in *' domstate '*) echo running;; *' reboot '*) echo rebooted;; esac\nexit 0\n")
		if err := runRestart([]string{configPath}); err != nil {
			t.Fatalf("runRestart: %v", err)
		}
		data, _ := os.ReadFile(logPath)
		if !strings.Contains(string(data), "reboot\ncmd-test") {
			t.Fatalf("virsh reboot not called:\n%s", data)
		}
	})
}

func TestSSHSCPDryRun(t *testing.T) {
	configPath, _, _ := commandFixture(t)
	fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; esac\nexit 0\n")

	out, err := captureCommandOutput(t, func() error {
		return runSSH([]string{"--dry-run", "--timeout", "0", "--command", "uname -a", configPath})
	})
	if err != nil {
		t.Fatalf("runSSH: %v", err)
	}
	for _, want := range []string{"ssh", "root@192.0.2.10", "uname -a"} {
		if !strings.Contains(out, want) {
			t.Errorf("ssh dry run missing %q:\n%s", want, out)
		}
	}

	local := filepath.Join(t.TempDir(), "app.conf")
	if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}
	out, err = captureCommandOutput(t, func() error {
		return runSCP([]string{"--dry-run", "--timeout", "0", local, "/etc/app.conf", configPath})
	})
	if err != nil {
		t.Fatalf("runSCP: %v", err)
	}
	for _, want := range []string{"scp", "root@192.0.2.10:/etc/app.conf"} {
		if !strings.Contains(out, want) {
			t.Errorf("scp dry run missing %q:\n%s", want, out)
		}
	}
}

func TestConsoleVNC(t *testing.T) {
	configPath, stateDir, _ := commandFixture(t)
	fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; *' domdisplay '*) echo vnc://127.0.0.1:0;; esac\nexit 0\n")
	out, err := captureCommandOutput(t, func() error {
		return runConsole([]string{"--state-dir", stateDir, "--type", "vnc", configPath})
	})
	if err != nil {
		t.Fatalf("runConsole: %v", err)
	}
	if !strings.Contains(out, "VNC: vnc://127.0.0.1:0") {
		t.Fatalf("console output = %q", out)
	}
}

func TestListJSON(t *testing.T) {
	_, stateDir, _ := commandFixture(t)
	fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; esac\nexit 0\n")
	out, err := captureCommandOutput(t, func() error {
		return runList([]string{"--state-dir", stateDir, "--json"})
	})
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	var entries []listEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("parse list JSON: %v\n%s", err, out)
	}
	if len(entries) != 1 || entries[0].Name != "cmd-test" || !entries[0].Running {
		t.Fatalf("entries = %+v", entries)
	}
}

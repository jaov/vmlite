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

func TestStartAndStopByVMName(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		configPath, stateDir, _ := commandFixture(t)
		if err := os.Remove(configPath); err != nil {
			t.Fatalf("remove config: %v", err)
		}
		logPath := filepath.Join(t.TempDir(), "virsh.log")
		fakeVirsh(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+logPath+"\ncase \"$*\" in *' domstate '*) echo 'shut off';; *' start '*) echo started;; esac\nexit 0\n")
		if err := runStart([]string{"--state-dir", stateDir, "cmd-test"}); err != nil {
			t.Fatalf("runStart by name: %v", err)
		}
		data, _ := os.ReadFile(logPath)
		if !strings.Contains(string(data), "start\ncmd-test") {
			t.Fatalf("virsh start not called by name:\n%s", data)
		}
	})

	t.Run("stop", func(t *testing.T) {
		configPath, stateDir, _ := commandFixture(t)
		if err := os.Remove(configPath); err != nil {
			t.Fatalf("remove config: %v", err)
		}
		logPath := filepath.Join(t.TempDir(), "virsh.log")
		stopFlag := filepath.Join(t.TempDir(), "stopped")
		t.Setenv("STOP_FLAG", stopFlag)
		fakeVirsh(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+logPath+"\ncase \"$*\" in *' domstate '*) if [ -f \"$STOP_FLAG\" ]; then echo 'shut off'; else echo running; fi;; *' shutdown '*) : > \"$STOP_FLAG\"; echo shutdown;; esac\nexit 0\n")
		if err := runStop([]string{"--state-dir", stateDir, "--timeout", "2s", "cmd-test"}); err != nil {
			t.Fatalf("runStop by name: %v", err)
		}
		data, _ := os.ReadFile(logPath)
		if !strings.Contains(string(data), "shutdown\ncmd-test") {
			t.Fatalf("virsh shutdown not called by name:\n%s", data)
		}
	})
}

func TestPowerUnknownVMName(t *testing.T) {
	_, stateDir, _ := commandFixture(t)
	err := runStop([]string{"--state-dir", stateDir, "missing-vm"})
	if err == nil || !strings.Contains(err.Error(), "no VM named") {
		t.Fatalf("runStop = %v, want unknown VM error", err)
	}
}

func TestNameTargets(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		configPath, _, _ := commandFixture(t)
		chdir(t, filepath.Dir(configPath))
		fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; esac\nexit 0\n")
		out, err := captureCommandOutput(t, func() error {
			return runStatus([]string{"cmd-test"})
		})
		if err != nil {
			t.Fatalf("status by name: %v", err)
		}
		if !strings.Contains(out, "Name:  cmd-test") {
			t.Fatalf("status output:\n%s", out)
		}
	})

	t.Run("inspect", func(t *testing.T) {
		configPath, _, _, _ := inspectFixture(t)
		chdir(t, filepath.Dir(configPath))
		fakeInspectTools(t)
		out, err := captureCommandOutput(t, func() error {
			return runInspect([]string{"--json", "cmd-test"})
		})
		if err != nil {
			t.Fatalf("inspect by name: %v", err)
		}
		var report inspectReport
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatalf("parse inspect JSON: %v\n%s", err, out)
		}
		if report.Name != "cmd-test" {
			t.Fatalf("report = %+v", report)
		}
	})

	t.Run("ssh", func(t *testing.T) {
		configPath, _, _ := commandFixture(t)
		chdir(t, filepath.Dir(configPath))
		fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; esac\nexit 0\n")
		out, err := captureCommandOutput(t, func() error {
			return runSSH([]string{"--dry-run", "--timeout", "0", "--command", "true", "cmd-test"})
		})
		if err != nil {
			t.Fatalf("ssh by name: %v", err)
		}
		if !strings.Contains(out, "root@192.0.2.10") {
			t.Fatalf("ssh output:\n%s", out)
		}
	})

	t.Run("scp", func(t *testing.T) {
		configPath, _, _ := commandFixture(t)
		chdir(t, filepath.Dir(configPath))
		fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; esac\nexit 0\n")
		local := filepath.Join(t.TempDir(), "payload")
		if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
			t.Fatalf("write local: %v", err)
		}
		out, err := captureCommandOutput(t, func() error {
			return runSCP([]string{"--dry-run", "--timeout", "0", local, "/tmp/payload", "cmd-test"})
		})
		if err != nil {
			t.Fatalf("scp by name: %v", err)
		}
		if !strings.Contains(out, "root@192.0.2.10:/tmp/payload") {
			t.Fatalf("scp output:\n%s", out)
		}
	})

	t.Run("console", func(t *testing.T) {
		configPath, _, _ := commandFixture(t)
		chdir(t, filepath.Dir(configPath))
		fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; *' domdisplay '*) echo vnc://127.0.0.1:0;; esac\nexit 0\n")
		out, err := captureCommandOutput(t, func() error {
			return runConsole([]string{"--type", "vnc", "cmd-test"})
		})
		if err != nil {
			t.Fatalf("console by name: %v", err)
		}
		if !strings.Contains(out, "VNC: vnc://127.0.0.1:0") {
			t.Fatalf("console output:\n%s", out)
		}
	})

	t.Run("provision", func(t *testing.T) {
		configPath, _, _ := commandFixture(t)
		chdir(t, filepath.Dir(configPath))
		if err := runProvision([]string{"cmd-test"}); err != nil {
			t.Fatalf("provision by name: %v", err)
		}
	})

	t.Run("destroy", func(t *testing.T) {
		configPath, _, stateFile := commandFixture(t)
		chdir(t, filepath.Dir(configPath))
		fakeVirsh(t, "#!/bin/sh\nexit 1\n")
		if err := runDestroy([]string{"--force", "cmd-test"}); err != nil {
			t.Fatalf("destroy by name: %v", err)
		}
		if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
			t.Fatalf("state file still exists: %v", err)
		}
	})
}

func TestConfigureAutostart(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "virsh.log")
	fakeVirsh(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" >> "+logPath+"\nexit 0\n")

	cfg := &Config{Name: "cmd-test", Autostart: true}
	if err := configureAutostart(cfg, systemURI, false); err != nil {
		t.Fatalf("configureAutostart: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read virsh log: %v", err)
	}
	if !strings.Contains(string(data), "autostart\ncmd-test") {
		t.Fatalf("virsh autostart not called:\n%s", data)
	}

	cfg.Autostart = false
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("remove log: %v", err)
	}
	if err := configureAutostart(cfg, systemURI, false); err != nil {
		t.Fatalf("configureAutostart disabled: %v", err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("virsh called with autostart disabled: %v", err)
	}
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

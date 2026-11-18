package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
)

func TestStatusFillsLiveIP(t *testing.T) {
	configPath, stateDir, stateFile := commandFixture(t)
	st, err := loadState(stateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	st.IP = ""
	if err := saveState(stateFile, st); err != nil {
		t.Fatalf("save state: %v", err)
	}
	fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; *' domifaddr '*) echo ' vnet0 52:54:00:aa:bb:cc ipv4 192.0.2.77/24';; esac\nexit 0\n")

	out, err := captureCommandOutput(t, func() error {
		return runStatus([]string{"--json", "--state-dir", stateDir, configPath})
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var status statusJSON
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("parse status JSON: %v\n%s", err, out)
	}
	if status.IP != "192.0.2.77" {
		t.Fatalf("status IP = %q", status.IP)
	}
	saved, err := loadState(stateFile)
	if err != nil {
		t.Fatalf("load saved state: %v", err)
	}
	if saved.IP != "192.0.2.77" {
		t.Fatalf("saved IP = %q", saved.IP)
	}
}

func TestStatusJSONNoState(t *testing.T) {
	path := writeConfigFile(t, `{
		"name": "status-json",
		"memory": 512,
		"cpus": 1,
		"source": {"type": "none"},
		"disk": {"size": 5}
	}`)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	runErr := runStatus([]string{"--json", path})
	w.Close()
	os.Stdout = old

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read status JSON: %v", err)
	}
	if !errors.Is(runErr, errNotRunning) {
		t.Fatalf("runStatus = %v, want errNotRunning", runErr)
	}
	var status statusJSON
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("parse status JSON: %v\n%s", err, data)
	}
	if status.Name != "status-json" || status.State != "not created" || status.Running {
		t.Fatalf("status = %+v", status)
	}
}

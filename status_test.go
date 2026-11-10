package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
)

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

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vmlite/internal/plan"
)

func TestEnsureSSHKeyGenerated(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not available")
	}
	layout := projectLayout{AssetDir: t.TempDir()}
	cfg := &Config{
		Name:         "dev",
		Source:       Source{Type: sourceCloudImage, Path: "/tmp/base.qcow2"},
		Provisioners: []Provisioner{{Type: "shell", Inline: "true"}},
	}
	if err := ensureSSHKey(cfg, layout); err != nil {
		t.Fatalf("ensureSSHKey: %v", err)
	}
	if cfg.SSH.Key == "" {
		t.Fatal("generated key path is empty")
	}
	if _, err := os.Stat(cfg.SSH.Key); err != nil {
		t.Fatalf("private key missing: %v", err)
	}
	if !strings.HasPrefix(cfg.sshPublicKey, "ssh-ed25519 ") {
		t.Fatalf("public key = %q", cfg.sshPublicKey)
	}
	if !cfg.sshKeyGenerated {
		t.Fatal("sshKeyGenerated = false")
	}
}

func TestProvisionShellStep(t *testing.T) {
	binDir := t.TempDir()
	script := "#!/bin/sh\n/bin/cat >/dev/null\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", binDir)

	state := &State{Version: 1, Name: "dev", Provisioned: nil}
	stateFile := filepath.Join(t.TempDir(), "state.json")
	runner := &plan.SSHRunner{Host: "192.0.2.10", User: "vmlite", Key: "/tmp/key"}
	step := &shellStep{
		desc:      "once",
		id:        "inline-0",
		script:    []byte("echo hello\n"),
		hash:      "abc",
		run:       "once",
		state:     state,
		stateFile: stateFile,
		runner:    runner,
	}

	ok, err := step.Check(context.Background(), runner)
	if err != nil || ok {
		t.Fatalf("Check before apply = %v, %v; want false, nil", ok, err)
	}
	if err := step.Apply(context.Background(), runner); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(state.Provisioned) != 1 || state.Provisioned[0].SHA256 != "abc" {
		t.Fatalf("state provisioned = %+v", state.Provisioned)
	}
	ok, err = step.Check(context.Background(), runner)
	if err != nil || !ok {
		t.Fatalf("Check after apply = %v, %v; want true, nil", ok, err)
	}

	state2 := &State{Version: 1, Name: "dev"}
	always := &shellStep{
		desc: "always", id: "inline-1", script: []byte("true"), hash: "def", run: "always",
		state: state2, stateFile: stateFile, runner: runner,
	}
	if ok, _ := always.Check(context.Background(), runner); ok {
		t.Fatal("always provisioner was skipped")
	}
}

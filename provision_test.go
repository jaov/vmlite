package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestEjectInstallerMedia(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "args.log")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> " + logPath + "\n" +
		"case \" $* \" in\n" +
		"  *\" domblklist \"*)\n" +
		"    printf ' Type   Device   Target   Source\\n'\n" +
		"    printf ' file   disk     vda      /tmp/disk.qcow2\\n'\n" +
		"    printf ' file   cdrom    sda      /tmp/ubuntu.iso\\n'\n" +
		"    printf ' file   cdrom    sdb      -\\n'\n" +
		"    ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake virsh: %v", err)
	}
	t.Setenv("PATH", binDir)

	if err := ejectInstallerMedia(systemURI, "demo", false); err != nil {
		t.Fatalf("ejectInstallerMedia: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	log := string(data)
	for _, want := range []string{"domblklist", "--details", "change-media\ndemo\nsda\n--eject\n--config"} {
		if !strings.Contains(log, want) {
			t.Errorf("virsh log missing %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "sdb") {
		t.Errorf("virsh attempted to eject already-empty sdb:\n%s", log)
	}
}

func TestClearInstallerBoot(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "args.log")
	definedPath := filepath.Join(binDir, "defined.xml")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> " + logPath + "\n" +
		"last=\"\"\n" +
		"for a in \"$@\"; do last=\"$a\"; done\n" +
		"case \" $* \" in\n" +
		"  *\" dumpxml \"*)\n" +
		"    printf '<domain><name>demo</name><os><kernel>/tmp/vmlinuz</kernel><initrd>/tmp/initrd</initrd><cmdline>autoinstall</cmdline></os><on_reboot>destroy</on_reboot><devices/></domain>\\n'\n" +
		"    ;;\n" +
		"  *\" define \"*)\n" +
		"    /bin/cp \"$last\" " + definedPath + "\n" +
		"    ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake virsh: %v", err)
	}
	t.Setenv("PATH", binDir)

	if err := clearInstallerBoot(systemURI, "demo"); err != nil {
		t.Fatalf("clearInstallerBoot: %v", err)
	}
	data, err := os.ReadFile(definedPath)
	if err != nil {
		t.Fatalf("read defined XML: %v", err)
	}
	xml := string(data)
	for _, gone := range []string{"<kernel", "<initrd", "<cmdline", "<on_reboot"} {
		if strings.Contains(xml, gone) {
			t.Errorf("defined XML still contains %q:\n%s", gone, xml)
		}
	}
	if !strings.Contains(xml, "<name>demo</name>") {
		t.Errorf("defined XML lost the domain:\n%s", xml)
	}
}

func TestEnsureSSHKeyRequiresUserKey(t *testing.T) {
	for _, sourceType := range []string{sourceISO, sourceDisk} {
		cfg := &Config{
			Name:         "dev",
			Source:       Source{Type: sourceType, Path: "/tmp/source"},
			Provisioners: []Provisioner{{Type: "shell", Inline: "true"}},
		}
		err := ensureSSHKey(cfg, projectLayout{AssetDir: t.TempDir()})
		if err == nil || !strings.Contains(err.Error(), "ssh.key is required") {
			t.Fatalf("source %q: error = %v, want ssh.key requirement", sourceType, err)
		}
	}
}

func TestWaitForIPv4PrefersARP(t *testing.T) {
	binDir := t.TempDir()
	script := "#!/bin/sh\n" +
		"case \" $* \" in\n" +
		"  *\" --source arp \"*) printf ' vnet0 52:54:00:00:00:01 ipv4 192.0.2.57/24\\n' ;;\n" +
		"  *\" --source lease \"*) printf ' vnet0 52:54:00:00:00:01 ipv4 192.0.2.56/24\\n vnet0 52:54:00:00:00:01 ipv4 192.0.2.57/24\\n' ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake virsh: %v", err)
	}
	t.Setenv("PATH", binDir)

	got, err := waitForIPv4(systemURI, "demo", 2*time.Second)
	if err != nil {
		t.Fatalf("waitForIPv4: %v", err)
	}
	if got != "192.0.2.57" {
		t.Fatalf("IP = %q, want active ARP address", got)
	}
}

func TestShellStepRootSkipsSudo(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "ssh.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + logPath + "\n/bin/cat >/dev/null\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", binDir)

	state := &State{Version: 1, Name: "dev"}
	runner := &plan.SSHRunner{Host: "192.0.2.10", User: "root", Key: "/tmp/key"}
	step := &shellStep{
		desc:      "root",
		id:        "inline-0",
		script:    []byte("echo hi\n"),
		hash:      "abc",
		run:       "once",
		state:     state,
		stateFile: filepath.Join(t.TempDir(), "state.json"),
		runner:    runner,
	}
	if err := step.Apply(context.Background(), runner); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read ssh log: %v", err)
	}
	if strings.Contains(string(data), "sudo") {
		t.Fatalf("root shell step should not use sudo:\n%s", data)
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

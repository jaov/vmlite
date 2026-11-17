package provision

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
	if err := EnsureSSHKey(cfg, layout); err != nil {
		t.Fatalf("ensureSSHKey: %v", err)
	}
	if cfg.SSH.Key == "" {
		t.Fatal("generated key path is empty")
	}
	if _, err := os.Stat(cfg.SSH.Key); err != nil {
		t.Fatalf("private key missing: %v", err)
	}
	if !strings.HasPrefix(cfg.SSHPublicKey, "ssh-ed25519 ") {
		t.Fatalf("public key = %q", cfg.SSHPublicKey)
	}
	if !cfg.SSHKeyGenerated {
		t.Fatal("SSHKeyGenerated = false")
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

	if err := EjectInstallerMedia(systemURI, "demo", false); err != nil {
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

	if err := ClearInstallerBoot(systemURI, "demo"); err != nil {
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
		err := EnsureSSHKey(cfg, projectLayout{AssetDir: t.TempDir()})
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

	got, err := WaitForIPv4(systemURI, "demo", 2*time.Second)
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

func TestFinishInstallerSSHCompletionSkipsShutdown(t *testing.T) {
	st := &State{
		Version:             1,
		Name:                "win",
		InstallerType:       "windows_autounattend",
		InstallerCompletion: "ssh",
		InstallPhase:        installPhaseInstalling,
	}
	if err := FinishInstaller(&Config{}, st, projectLayout{}, false); err != nil {
		t.Fatalf("finishInstaller: %v", err)
	}
}

func TestProvisionTimeouts(t *testing.T) {
	cases := []struct {
		name     string
		st       *State
		override time.Duration
		wantIP   time.Duration
		wantSSH  time.Duration
	}{
		{"default", &State{}, 0, 5 * time.Minute, 5 * time.Minute},
		{"installer", &State{InstallerType: "ubuntu_autoinstall"}, 0, 5 * time.Minute, 10 * time.Minute},
		{"ssh installer", &State{InstallerType: "windows_autounattend", InstallerCompletion: "ssh"}, 0, 30 * time.Minute, 30 * time.Minute},
		{"override", &State{InstallerType: "windows_autounattend", InstallerCompletion: "ssh"}, 45 * time.Minute, 45 * time.Minute, 45 * time.Minute},
	}
	for _, tc := range cases {
		gotIP, gotSSH := ProvisionTimeouts(tc.st, tc.override)
		if gotIP != tc.wantIP || gotSSH != tc.wantSSH {
			t.Errorf("%s: timeouts = %s/%s, want %s/%s", tc.name, gotIP, gotSSH, tc.wantIP, tc.wantSSH)
		}
	}
}

func TestPowerShellStep(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "ssh.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + logPath + "\n/bin/cat >/dev/null\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Setenv("PATH", binDir)

	state := &State{Version: 1, Name: "win"}
	runner := &plan.SSHRunner{Host: "192.0.2.10", User: "Administrator", Key: "/tmp/key"}
	step := &shellStep{
		desc:      "powershell",
		id:        "inline-0",
		typ:       "powershell",
		script:    []byte("Write-Output 'hi'\n"),
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
	log := string(data)
	for _, want := range []string{"powershell", "-NoProfile", "-NonInteractive", "-Command"} {
		if !strings.Contains(log, want) {
			t.Errorf("ssh log missing %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "sudo") {
		t.Fatalf("PowerShell step should not use sudo:\n%s", log)
	}
}

func TestShellStepUploadsAndEnv(t *testing.T) {
	binDir := t.TempDir()
	sshLog := filepath.Join(binDir, "ssh.log")
	scpLog := filepath.Join(binDir, "scp.log")
	sshScript := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + sshLog + "\n/bin/cat >/dev/null\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "ssh"), []byte(sshScript), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	scpScript := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + scpLog + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "scp"), []byte(scpScript), 0o755); err != nil {
		t.Fatalf("write fake scp: %v", err)
	}
	t.Setenv("PATH", binDir)

	local := filepath.Join(t.TempDir(), "app.conf")
	data := []byte("setting=1\n")
	if err := os.WriteFile(local, data, 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	state := &State{Version: 1, Name: "dev"}
	runner := &plan.SSHRunner{Host: "192.0.2.10", User: "root", Key: "/tmp/key"}
	step := &shellStep{
		desc:      "uploads",
		id:        "inline-0",
		typ:       "shell",
		script:    []byte("echo hi\n"),
		hash:      "abc",
		run:       "once",
		state:     state,
		stateFile: filepath.Join(t.TempDir(), "state.json"),
		runner:    runner,
		uploads:   []provisionUpload{{source: local, target: "/etc/app.conf", data: data}},
		env:       map[string]string{"APP_ENV": "dev", "RETRIES": "3"},
	}
	if err := step.Apply(context.Background(), runner); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	scpData, err := os.ReadFile(scpLog)
	if err != nil {
		t.Fatalf("read scp log: %v", err)
	}
	scp := string(scpData)
	for _, want := range []string{local, "root@192.0.2.10:/etc/app.conf"} {
		if !strings.Contains(scp, want) {
			t.Errorf("scp log missing %q:\n%s", want, scp)
		}
	}
	sshData, err := os.ReadFile(sshLog)
	if err != nil {
		t.Fatalf("read ssh log: %v", err)
	}
	ssh := string(sshData)
	for _, want := range []string{"env", "APP_ENV=dev", "RETRIES=3", "bash", "-s"} {
		if !strings.Contains(ssh, want) {
			t.Errorf("ssh log missing %q:\n%s", want, ssh)
		}
	}
}

func TestPowerShellEnvPrelude(t *testing.T) {
	got := powershellScriptWithEnv([]byte("Write-Output hi\n"), map[string]string{"APP_ENV": "dev", "QUOTED": "a'b"})
	script := string(got)
	for _, want := range []string{"$env:APP_ENV = 'dev'", "$env:QUOTED = 'a''b'", "Write-Output hi"} {
		if !strings.Contains(script, want) {
			t.Errorf("PowerShell script missing %q:\n%s", want, script)
		}
	}
}

func TestProvisionHashIncludesEnvAndUploads(t *testing.T) {
	base := provisionHash([]byte("echo hi"), nil, nil)
	withEnv := provisionHash([]byte("echo hi"), map[string]string{"APP_ENV": "dev"}, nil)
	withUpload := provisionHash([]byte("echo hi"), nil, []provisionUpload{{source: "/tmp/a", target: "/etc/a", data: []byte("a")}})
	if base == withEnv || base == withUpload || withEnv == withUpload {
		t.Fatalf("hashes should differ: %q %q %q", base, withEnv, withUpload)
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

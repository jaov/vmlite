package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeInspectTools(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	virsh := `#!/bin/sh
case "$*" in
  *' domstate '*) echo running ;;
  *' domuuid '*) echo 11111111-2222-3333-4444-555555555555 ;;
  *' dumpxml '*)
    cat <<'EOF'
<domain type='kvm'>
  <uuid>11111111-2222-3333-4444-555555555555</uuid>
  <memory unit='KiB'>524288</memory>
  <vcpu>2</vcpu>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='/tmp/disk.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='/tmp/seed.iso'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
    <interface type='network'>
      <mac address='52:54:00:aa:bb:cc'/>
      <source network='default'/>
      <model type='virtio'/>
      <target dev='vnet0'/>
    </interface>
  </devices>
</domain>
EOF
    ;;
  *' domifaddr '*)
    echo " vnet0 52:54:00:aa:bb:cc ipv4 192.0.2.20/24"
    ;;
  *' domstats '*)
    echo "Domain: 'cmd-test'"
    echo "state.state=1"
    echo "cpu.time=1500000000"
    ;;
  *' domblkinfo '*)
    echo "Capacity: 10737418240"
    echo "Allocation: 2147483648"
    echo "Physical: 2147483648"
    ;;
esac
exit 0
`
	qemuImg := `#!/bin/sh
echo '[{"filename":"/tmp/disk.qcow2","format":"qcow2","virtual-size":10737418240,"actual-size":2147483648,"full-backing-filename":"/tmp/base.qcow2"}]'
`
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(virsh), 0o755); err != nil {
		t.Fatalf("write fake virsh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "qemu-img"), []byte(qemuImg), 0o755); err != nil {
		t.Fatalf("write fake qemu-img: %v", err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	return binDir
}

func inspectFixture(t *testing.T) (configPath, stateDir, stateFile, assetDir string) {
	t.Helper()
	configPath, stateDir, stateFile = commandFixture(t)
	assetDir = filepath.Join(stateDir, "cmd-test")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("create asset dir: %v", err)
	}
	return configPath, stateDir, stateFile, assetDir
}

func TestInspectReport(t *testing.T) {
	_, stateDir, stateFile, assetDir := inspectFixture(t)
	fakeInspectTools(t)
	st, err := loadState(stateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	layout := projectLayout{
		ConfigPath: stateFile,
		StateDir:   stateDir,
		StateFile:  stateFile,
		AssetDir:   assetDir,
	}
	report := buildInspectReport(st, layout)
	if report.Name != "cmd-test" || report.State != "running" || !report.Running {
		t.Fatalf("report = %+v", report)
	}
	if report.MemoryMiB != 512 || report.VCPUs != 2 {
		t.Fatalf("cpu/memory = %d/%d", report.VCPUs, report.MemoryMiB)
	}
	if report.UUID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("uuid = %q", report.UUID)
	}
	if len(report.Disks) != 1 {
		t.Fatalf("disks = %+v", report.Disks)
	}
	disk := report.Disks[0]
	if disk.Target != "vda" || disk.Format != "qcow2" || disk.Capacity != "10.000 GiB" || disk.Backing != "/tmp/base.qcow2" {
		t.Fatalf("disk = %+v", disk)
	}
	if len(report.Media) != 1 || !report.Media[0].ReadOnly || report.Media[0].Target != "sda" {
		t.Fatalf("media = %+v", report.Media)
	}
	if len(report.Network) != 1 || report.Network[0].MAC != "52:54:00:aa:bb:cc" || report.Network[0].Network != "default" {
		t.Fatalf("network = %+v", report.Network)
	}
	if !containsString(report.Addresses, "192.0.2.20") || !containsString(report.Addresses, "192.0.2.10") {
		t.Fatalf("addresses = %+v", report.Addresses)
	}
	if report.Stats["cpu.time"] != "1500000000" {
		t.Fatalf("stats = %+v", report.Stats)
	}
	if report.Paths.State != stateFile || report.Paths.Assets != assetDir {
		t.Fatalf("paths = %+v", report.Paths)
	}
}

func TestInspectJSONAndXML(t *testing.T) {
	configPath, stateDir, _, _ := inspectFixture(t)
	fakeInspectTools(t)

	out, err := captureCommandOutput(t, func() error {
		return runInspect([]string{"--json", "--state-dir", stateDir, configPath})
	})
	if err != nil {
		t.Fatalf("inspect --json: %v", err)
	}
	var report inspectReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("parse inspect JSON: %v\n%s", err, out)
	}
	if report.Name != "cmd-test" || !report.Running || len(report.Disks) != 1 {
		t.Fatalf("report = %+v", report)
	}

	xmlOut, err := captureCommandOutput(t, func() error {
		return runInspect([]string{"--xml", "--state-dir", stateDir, configPath})
	})
	if err != nil {
		t.Fatalf("inspect --xml: %v", err)
	}
	if !strings.Contains(xmlOut, "<domain") || !strings.Contains(xmlOut, "<uuid>") {
		t.Fatalf("xml output = %q", xmlOut)
	}
}

func TestInspectRejectsJSONAndXML(t *testing.T) {
	configPath, stateDir, _, _ := inspectFixture(t)
	err := runInspect([]string{"--json", "--xml", "--state-dir", stateDir, configPath})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("inspect = %v", err)
	}
}

func TestConsoleLogPath(t *testing.T) {
	st := &State{AssetDir: "/tmp/vmlite/dev"}
	layout := projectLayout{AssetDir: "/tmp/vmlite/fallback"}
	if got := consoleLogPath(st, layout, ""); got != "/tmp/vmlite/dev/console.log" {
		t.Fatalf("default log path = %q", got)
	}
	if got := consoleLogPath(st, layout, "/tmp/custom.log"); got != "/tmp/custom.log" {
		t.Fatalf("custom log path = %q", got)
	}
	if got := consoleLogPath(&State{}, layout, ""); got != "/tmp/vmlite/fallback/console.log" {
		t.Fatalf("fallback log path = %q", got)
	}
}

func TestConsoleLogDryRun(t *testing.T) {
	configPath, stateDir, _, _ := inspectFixture(t)
	fakeVirsh(t, "#!/bin/sh\ncase \"$*\" in *' domstate '*) echo running;; *' domdisplay '*) echo;; esac\nexit 0\n")
	out, err := captureCommandOutput(t, func() error {
		return runConsole([]string{"--dry-run", "--type", "serial", "--log", "--state-dir", stateDir, configPath})
	})
	if err != nil {
		t.Fatalf("console --log --dry-run: %v", err)
	}
	if !strings.Contains(out, "script -q -c") || !strings.Contains(out, "virsh") || !strings.Contains(out, "console.log") {
		t.Fatalf("console dry run = %q", out)
	}
}

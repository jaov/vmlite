package libvirt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeVirsh(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *' domstate missing'*) echo "error: failed to get domain" >&2; exit 1 ;;
  *' domstate '*) echo running ;;
  *' domuuid '*) echo 11111111-2222-3333-4444-555555555555 ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "virsh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake virsh: %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestRunStatusUUID(t *testing.T) {
	fakeVirsh(t)
	if out, err := Run("qemu:///session", "domstate", "vm"); err != nil || !strings.Contains(out, "running") {
		t.Fatalf("Run = %q, %v", out, err)
	}
	if state, ok := Status("qemu:///session", "vm"); !ok || state != "running" {
		t.Fatalf("Status = %q, %v", state, ok)
	}
	if _, ok := Status("qemu:///session", "missing"); ok {
		t.Fatal("missing domain reported as present")
	}
	if got := UUID("qemu:///session", "vm"); got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("UUID = %q", got)
	}
}

func TestMemoryMiB(t *testing.T) {
	cases := []struct {
		unit  string
		value int64
		want  int64
	}{
		{"", 524288, 512},
		{"KiB", 524288, 512},
		{"MiB", 512, 512},
		{"GiB", 1, 1024},
		{"TiB", 1, 1024 * 1024},
	}
	for _, tc := range cases {
		var d DomainXML
		d.Memory.Unit = tc.unit
		d.Memory.Value = tc.value
		got, err := MemoryMiB(d)
		if err != nil {
			t.Fatalf("%q: %v", tc.unit, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %d, want %d", tc.unit, got, tc.want)
		}
	}
	var d DomainXML
	d.Memory.Unit = "B"
	if _, err := MemoryMiB(d); err == nil {
		t.Fatal("expected unsupported unit error")
	}
}

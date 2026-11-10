package seed

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeISOBuilder(t *testing.T) (binDir, argsLog string) {
	t.Helper()
	binDir = t.TempDir()
	argsLog = filepath.Join(binDir, "args.log")
	script := fmt.Sprintf(`#!/bin/sh
out=""
prev=""
last=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out="$a"; fi
  prev="$a"
  last="$a"
done
printf '%%s\n' "$@" > %q
for f in "$last"/*; do
  printf 'FILE=%%s\n' "${f##*/}" >> %q
  /bin/cat "$f" >> %q
done
: > "$out"
exit 0
`, argsLog, argsLog, argsLog)
	if err := os.WriteFile(filepath.Join(binDir, "xorrisofs"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake xorrisofs: %v", err)
	}
	t.Setenv("PATH", binDir)
	return binDir, argsLog
}

func TestWriteISO(t *testing.T) {
	_, argsLog := fakeISOBuilder(t)
	path := filepath.Join(t.TempDir(), "seed.iso")
	volume := Volume{
		Label: "cidata",
		Files: []File{
			{Name: "user-data", Data: []byte("#cloud-config\n")},
			{Name: "meta-data", Data: []byte("instance-id: demo\n")},
		},
	}
	if err := WriteISO(volume, path); err != nil {
		t.Fatalf("WriteISO: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("seed not created: %v", err)
	}

	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	log := string(data)
	for _, want := range []string{"-o\n" + path, "-V\ncidata", "FILE=meta-data", "FILE=user-data", "instance-id: demo"} {
		if !strings.Contains(log, want) {
			t.Errorf("builder log missing %q:\n%s", want, log)
		}
	}
}

func TestWriteISORejectsBadFile(t *testing.T) {
	fakeISOBuilder(t)
	err := WriteISO(Volume{
		Label: "cidata",
		Files: []File{{Name: "../escape", Data: []byte("x")}},
	}, filepath.Join(t.TempDir(), "seed.iso"))
	if err == nil || !strings.Contains(err.Error(), "plain filename") {
		t.Fatalf("error = %v, want plain filename", err)
	}
}

func TestWriteISORequiresTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := WriteISO(Volume{
		Label: "cidata",
		Files: []File{{Name: "user-data", Data: []byte("x")}},
	}, filepath.Join(t.TempDir(), "seed.iso"))
	if err == nil || !strings.Contains(err.Error(), "xorrisofs") {
		t.Fatalf("error = %v, want builder error", err)
	}
}

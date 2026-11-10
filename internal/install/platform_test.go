package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRelease(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write os-release: %v", err)
	}
	return path
}

func TestDetectPlatformFamilies(t *testing.T) {
	tests := []struct {
		name    string
		release string
		want    string
	}{
		{"ubuntu", "ID=ubuntu\nID_LIKE=debian\n", "debian"},
		{"debian", "ID=debian\n", "debian"},
		{"fedora", "ID=fedora\n", "fedora"},
		{"rhel", "ID=rhel\nID_LIKE=\"fedora\"\n", "fedora"},
		{"arch", "ID=arch\n", "arch"},
		{"opensuse", "ID=opensuse-leap\nID_LIKE=\"suse\"\n", "suse"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := DetectPlatformFrom(writeRelease(t, tc.release))
			if err != nil {
				t.Fatalf("DetectPlatformFrom: %v", err)
			}
			if p.Family != tc.want {
				t.Fatalf("family = %q, want %q", p.Family, tc.want)
			}
		})
	}
}

func TestDetectPlatformRejectsFreeBSD(t *testing.T) {
	_, err := DetectPlatformFrom(writeRelease(t, "ID=freebsd\n"))
	if err == nil || !strings.Contains(err.Error(), "FreeBSD is not supported") {
		t.Fatalf("error = %v, want FreeBSD rejection", err)
	}
}

func TestDetectPlatformRejectsUnknown(t *testing.T) {
	_, err := DetectPlatformFrom(writeRelease(t, "ID=plan9\n"))
	if err == nil || !strings.Contains(err.Error(), "unsupported distribution") {
		t.Fatalf("error = %v, want unsupported error", err)
	}
}

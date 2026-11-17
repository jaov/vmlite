package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeQEMUImg(t *testing.T) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(binDir, "args.log")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nexit 0\n", logPath)
	if err := os.WriteFile(filepath.Join(binDir, "qemu-img"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake qemu-img: %v", err)
	}
	return binDir, logPath
}

func fakeQEMUImgInfo(t *testing.T, format string) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(binDir, "args.log")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "info" ]; then
  printf '{"format":"%s"}\n'
  exit 0
fi
printf '%%s\n' "$@" > %q
exit 0
`, format, logPath)
	if err := os.WriteFile(filepath.Join(binDir, "qemu-img"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake qemu-img: %v", err)
	}
	return binDir, logPath
}

func TestOverlayPathDefault(t *testing.T) {
	cfg := &Config{Name: "demo", Source: Source{Type: sourceCloudImage, Path: "/tmp/images/base.qcow2"}}

	got, err := cfg.OverlayPath()
	if err != nil {
		t.Fatalf("overlayPath: %v", err)
	}
	want := "/tmp/images/demo-overlay.qcow2"
	if got != want {
		t.Fatalf("overlayPath = %q, want %q", got, want)
	}
}

func TestOverlayPathExplicit(t *testing.T) {
	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: "/tmp/base.qcow2"},
		Disk:   Disk{Path: "/var/tmp/demo.qcow2"},
	}

	got, err := cfg.OverlayPath()
	if err != nil {
		t.Fatalf("overlayPath: %v", err)
	}
	if got != "/var/tmp/demo.qcow2" {
		t.Fatalf("overlayPath = %q", got)
	}
}

func TestSeedPath(t *testing.T) {
	cfg := &Config{Name: "demo", Source: Source{Type: sourceCloudImage, Path: "/tmp/images/base.qcow2"}}

	got, err := cfg.SeedPath()
	if err != nil {
		t.Fatalf("seedPath: %v", err)
	}
	want := "/tmp/images/demo-seed.iso"
	if got != want {
		t.Fatalf("seedPath = %q, want %q", got, want)
	}
}

func shaHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestResolveImageLocal(t *testing.T) {
	data := []byte("local-image")
	dir := t.TempDir()
	image := filepath.Join(dir, "image.qcow2")
	if err := os.WriteFile(image, data, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	cfg := &Config{Source: Source{Type: sourceCloudImage, Path: image, SHA256: shaHex(data)}}
	got, err := resolveImage(cfg, false, false)
	if err != nil {
		t.Fatalf("resolveImage: %v", err)
	}
	if got != image {
		t.Fatalf("path = %q, want %q", got, image)
	}

	cfg.Source.SHA256 = strings.Repeat("0", 64)
	if _, err := resolveImage(cfg, false, false); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("mismatch error = %v", err)
	}
}

func TestResolveMediaURL(t *testing.T) {
	data := []byte("installer-media")
	sha := shaHex(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	for _, sourceType := range []string{sourceCloudImage, sourceISO, sourceDisk} {
		cache := t.TempDir()
		cfg := &Config{Source: Source{
			Type:   sourceType,
			URL:    server.URL + "/media.iso",
			SHA256: sha,
			Cache:  cache,
		}}
		got, err := resolveImage(cfg, false, false)
		if err != nil {
			t.Fatalf("%s: resolveImage: %v", sourceType, err)
		}
		content, err := os.ReadFile(got)
		if err != nil || string(content) != string(data) {
			t.Fatalf("%s: cached media = %q, err %v", sourceType, content, err)
		}
	}
}

func TestResolveImageURL(t *testing.T) {
	data := []byte("remote-image")
	sha := shaHex(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	cache := t.TempDir()
	cfg := &Config{Source: Source{
		Type:   sourceCloudImage,
		URL:    server.URL + "/ubuntu.qcow2",
		SHA256: sha,
		Cache:  cache,
	}}

	got, err := resolveImage(cfg, false, false)
	if err != nil {
		t.Fatalf("resolveImage: %v", err)
	}
	content, err := os.ReadFile(got)
	if err != nil || string(content) != string(data) {
		t.Fatalf("cached image = %q, err %v", content, err)
	}
	if filepath.Dir(got) != cache {
		t.Fatalf("cache path = %q, want dir %q", got, cache)
	}

	// Cached image must work without the server.
	server.Close()
	if _, err := resolveImage(cfg, false, false); err != nil {
		t.Fatalf("cached resolveImage: %v", err)
	}
	if _, err := resolveImage(cfg, true, false); err != nil {
		t.Fatalf("offline cached resolveImage: %v", err)
	}
}

func TestResolveImageOfflineMissing(t *testing.T) {
	cfg := &Config{Source: Source{
		Type:   sourceCloudImage,
		URL:    "https://example.invalid/image.qcow2",
		SHA256: strings.Repeat("a", 64),
		Cache:  t.TempDir(),
	}}
	_, err := resolveImage(cfg, true, false)
	if err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("error = %v, want offline error", err)
	}
}

func TestResolveImageDryRunMissing(t *testing.T) {
	cfg := &Config{Source: Source{
		Type:   sourceCloudImage,
		URL:    "https://example.invalid/image.qcow2",
		SHA256: strings.Repeat("a", 64),
		Cache:  t.TempDir(),
	}}
	_, err := resolveImage(cfg, false, true)
	if err != errImageWouldDownload {
		t.Fatalf("error = %v, want errImageWouldDownload", err)
	}
}

func TestResolveImageChecksumMismatch(t *testing.T) {
	data := []byte("wrong")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	cfg := &Config{Source: Source{
		Type:   sourceCloudImage,
		URL:    server.URL + "/image.qcow2",
		SHA256: strings.Repeat("0", 64),
		Cache:  t.TempDir(),
	}}
	_, err := resolveImage(cfg, false, false)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
}

func TestPrepareOverlay(t *testing.T) {
	binDir, logPath := fakeQEMUImg(t)
	t.Setenv("PATH", binDir)

	base := filepath.Join(t.TempDir(), "base.qcow2")
	if err := os.WriteFile(base, []byte("base"), 0o644); err != nil {
		t.Fatalf("write base image: %v", err)
	}
	overlay := filepath.Join(t.TempDir(), "overlay.qcow2")

	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: base},
		Disk:   Disk{Size: 10, Path: overlay, Format: diskFormatQCOW2},
	}
	got, err := prepareOverlay(cfg)
	if err != nil {
		t.Fatalf("prepareOverlay: %v", err)
	}
	if got != overlay {
		t.Fatalf("overlay = %q, want %q", got, overlay)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	want := []string{"create", "-f", "qcow2", "-F", "qcow2", "-b", base, overlay, "10G"}
	if got := strings.Split(strings.TrimSpace(string(data)), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("qemu-img args = %q, want %q", got, want)
	}
}

func TestPrepareOverlayAutoBase(t *testing.T) {
	binDir, logPath := fakeQEMUImgInfo(t, diskFormatRaw)
	t.Setenv("PATH", binDir)

	base := filepath.Join(t.TempDir(), "base.img")
	if err := os.WriteFile(base, []byte("base"), 0o644); err != nil {
		t.Fatalf("write base image: %v", err)
	}
	overlay := filepath.Join(t.TempDir(), "overlay.qcow2")

	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: base},
		Disk:   Disk{Size: 10, Path: overlay, Format: diskFormatAuto},
	}
	if _, err := prepareOverlay(cfg); err != nil {
		t.Fatalf("prepareOverlay: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !containsAll(args, "-F", diskFormatRaw) {
		t.Fatalf("qemu-img args = %q, want -F raw", args)
	}
}

func TestPrepareOverlayRawBase(t *testing.T) {
	binDir, logPath := fakeQEMUImg(t)
	t.Setenv("PATH", binDir)

	base := filepath.Join(t.TempDir(), "base.raw")
	if err := os.WriteFile(base, []byte("base"), 0o644); err != nil {
		t.Fatalf("write base image: %v", err)
	}
	overlay := filepath.Join(t.TempDir(), "overlay.qcow2")

	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: base},
		Disk:   Disk{Size: 10, Path: overlay, Format: diskFormatRaw},
	}
	if _, err := prepareOverlay(cfg); err != nil {
		t.Fatalf("prepareOverlay: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	if !containsAll(args, "-F", diskFormatRaw) {
		t.Fatalf("qemu-img args = %q, want -F raw", args)
	}
}

func TestPrepareOverlayRefusesExistingDefault(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.qcow2")
	if err := os.WriteFile(base, []byte("base"), 0o644); err != nil {
		t.Fatalf("write base image: %v", err)
	}
	overlay := filepath.Join(dir, "demo-overlay.qcow2")
	if err := os.WriteFile(overlay, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	cfg := &Config{Name: "demo", Source: Source{Type: sourceCloudImage, Path: base}, Disk: Disk{Size: 10}}
	_, err := prepareOverlay(cfg)
	if err == nil {
		t.Fatal("prepareOverlay succeeded, want error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %q, want already exists", err)
	}
}

func TestPrepareOverlayReusesExplicitDiskPath(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.qcow2")
	if err := os.WriteFile(base, []byte("base"), 0o644); err != nil {
		t.Fatalf("write base image: %v", err)
	}
	overlay := filepath.Join(dir, "existing-overlay.qcow2")
	if err := os.WriteFile(overlay, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: base},
		Disk:   Disk{Size: 10, Path: overlay},
	}
	got, err := prepareOverlay(cfg)
	if err != nil {
		t.Fatalf("prepareOverlay: %v", err)
	}
	if got != overlay {
		t.Fatalf("overlay = %q, want %q", got, overlay)
	}
}

func TestPrepareOverlayMissingImage(t *testing.T) {
	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: filepath.Join(t.TempDir(), "missing.qcow2")},
		Disk:   Disk{Size: 10, Path: filepath.Join(t.TempDir(), "overlay.qcow2")},
	}

	_, err := prepareOverlay(cfg)
	if err == nil {
		t.Fatal("prepareOverlay succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("error = %q, want not accessible", err)
	}
}

func TestPrepareSeed(t *testing.T) {
	dir := t.TempDir()
	binDir := t.TempDir()
	argsLog := filepath.Join(binDir, "args.log")
	metaLog := filepath.Join(binDir, "meta.log")
	userLog := filepath.Join(binDir, "user.log")

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
/bin/cat "$last/meta-data" > %q
/bin/cat "$last/user-data" > %q
: > "$out"
exit 0
`, argsLog, metaLog, userLog)
	if err := os.WriteFile(filepath.Join(binDir, "xorrisofs"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake xorrisofs: %v", err)
	}
	t.Setenv("PATH", binDir)

	userData := filepath.Join(dir, "user-data")
	content := "#cloud-config\nusers:\n  - default\n"
	if err := os.WriteFile(userData, []byte(content), 0o644); err != nil {
		t.Fatalf("write user-data: %v", err)
	}

	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: filepath.Join(dir, "base.qcow2"), CloudInit: userData},
		Disk:   Disk{Path: filepath.Join(dir, "overlay.qcow2")},
	}
	seed, err := prepareSeed(cfg)
	if err != nil {
		t.Fatalf("prepareSeed: %v", err)
	}
	wantSeed := filepath.Join(dir, "demo-seed.iso")
	if seed != wantSeed {
		t.Fatalf("seed = %q, want %q", seed, wantSeed)
	}
	if _, err := os.Stat(seed); err != nil {
		t.Fatalf("seed was not created: %v", err)
	}

	argsData, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(argsData)), "\n")
	for _, want := range []string{"-o", wantSeed, "-J", "-input-charset", "utf8", "-rational-rock", "-V", "cidata"} {
		if !containsAll(args, want) {
			t.Errorf("iso args missing %q: %q", want, args)
		}
	}

	meta, err := os.ReadFile(metaLog)
	if err != nil {
		t.Fatalf("read meta-data: %v", err)
	}
	for _, want := range []string{"instance-id: demo", "local-hostname: demo"} {
		if !strings.Contains(string(meta), want) {
			t.Errorf("meta-data missing %q:\n%s", want, meta)
		}
	}

	gotUser, err := os.ReadFile(userLog)
	if err != nil {
		t.Fatalf("read user-data: %v", err)
	}
	if string(gotUser) != content {
		t.Errorf("seed user-data = %q, want %q", gotUser, content)
	}
}

func TestPrepareSeedReusesExisting(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "demo-seed.iso")
	if err := os.WriteFile(seed, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	t.Setenv("PATH", t.TempDir()) // no ISO tool available

	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: filepath.Join(dir, "base.qcow2"), CloudInit: filepath.Join(dir, "missing-user-data")},
		Disk:   Disk{Path: filepath.Join(dir, "overlay.qcow2")},
	}
	got, err := prepareSeed(cfg)
	if err != nil {
		t.Fatalf("prepareSeed: %v", err)
	}
	if got != seed {
		t.Fatalf("seed = %q, want %q", got, seed)
	}
}

func TestPrepareSeedMissingUserData(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Name:   "demo",
		Source: Source{Type: sourceCloudImage, Path: filepath.Join(dir, "base.qcow2"), CloudInit: filepath.Join(dir, "missing-user-data")},
		Disk:   Disk{Path: filepath.Join(dir, "overlay.qcow2")},
	}

	_, err := prepareSeed(cfg)
	if err == nil {
		t.Fatal("prepareSeed succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("error = %q, want not accessible", err)
	}
}

func TestSourceDiskFormat(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		for _, tc := range []struct {
			format string
			want   string
		}{
			{diskFormatQCOW2, diskFormatQCOW2},
			{diskFormatRaw, diskFormatRaw},
		} {
			cfg := &Config{Source: Source{Type: sourceDisk, Path: "/tmp/disk"}, Disk: Disk{Format: tc.format}}
			got, err := cfg.SourceDiskFormat()
			if err != nil {
				t.Fatalf("format %q: %v", tc.format, err)
			}
			if got != tc.want {
				t.Fatalf("format %q: got %q, want %q", tc.format, got, tc.want)
			}
		}
	})

	t.Run("auto", func(t *testing.T) {
		binDir, _ := fakeQEMUImgInfo(t, diskFormatRaw)
		t.Setenv("PATH", binDir)
		cfg := &Config{Source: Source{Type: sourceDisk, Path: "/tmp/disk"}, Disk: Disk{Format: diskFormatAuto}}
		got, err := cfg.SourceDiskFormat()
		if err != nil {
			t.Fatalf("sourceDiskFormat: %v", err)
		}
		if got != diskFormatRaw {
			t.Fatalf("format = %q, want %q", got, diskFormatRaw)
		}
	})
}

func TestExtraDiskFormat(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		for _, tc := range []struct {
			format string
			want   string
		}{
			{diskFormatRaw, diskFormatRaw},
			{diskFormatQCOW2, diskFormatQCOW2},
		} {
			got, err := extraDiskFormat(ExtraDisk{Format: tc.format, Path: "/tmp/disk"})
			if err != nil {
				t.Fatalf("format %q: %v", tc.format, err)
			}
			if got != tc.want {
				t.Fatalf("format %q: got %q, want %q", tc.format, got, tc.want)
			}
		}
	})

	t.Run("auto", func(t *testing.T) {
		binDir, _ := fakeQEMUImgInfo(t, diskFormatRaw)
		t.Setenv("PATH", binDir)
		got, err := extraDiskFormat(ExtraDisk{Format: diskFormatAuto, Path: "/tmp/config.iso"})
		if err != nil {
			t.Fatalf("extraDiskFormat: %v", err)
		}
		if got != diskFormatRaw {
			t.Fatalf("format = %q, want %q", got, diskFormatRaw)
		}
	})
}

func TestCheckImageTools(t *testing.T) {
	writeTool := func(t *testing.T, dir, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	t.Run("cloud image requires qemu-img", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		cfg := &Config{Source: Source{Type: sourceCloudImage, Path: "/tmp/base.qcow2"}}
		err := CheckImageTools(cfg)
		if err == nil || !strings.Contains(err.Error(), "qemu-img") {
			t.Fatalf("err = %v, want qemu-img error", err)
		}
	})

	t.Run("cloud init requires iso tool", func(t *testing.T) {
		dir := t.TempDir()
		writeTool(t, dir, "qemu-img")
		t.Setenv("PATH", dir)
		cfg := &Config{Source: Source{Type: sourceCloudImage, Path: "/tmp/base.qcow2", CloudInit: "/tmp/user-data"}}
		err := CheckImageTools(cfg)
		if err == nil || !strings.Contains(err.Error(), "xorrisofs") {
			t.Fatalf("err = %v, want ISO tool error", err)
		}
	})

	t.Run("tools present", func(t *testing.T) {
		dir := t.TempDir()
		writeTool(t, dir, "qemu-img")
		writeTool(t, dir, "genisoimage")
		t.Setenv("PATH", dir)
		cfg := &Config{Source: Source{Type: sourceCloudImage, Path: "/tmp/base.qcow2", CloudInit: "/tmp/user-data"}}
		if err := CheckImageTools(cfg); err != nil {
			t.Fatalf("CheckImageTools: %v", err)
		}
	})

	t.Run("none needs no tools", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if err := CheckImageTools(&Config{Source: Source{Type: sourceNone}}); err != nil {
			t.Fatalf("CheckImageTools: %v", err)
		}
	})

	t.Run("install tree needs no tools", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if err := CheckImageTools(&Config{Source: Source{Type: sourceInstallTree, URL: "https://example.invalid"}}); err != nil {
			t.Fatalf("CheckImageTools: %v", err)
		}
	})

	t.Run("source disk auto requires qemu-img", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		cfg := &Config{Source: Source{Type: sourceDisk, Path: "/tmp/disk"}, Disk: Disk{Format: diskFormatAuto}}
		err := CheckImageTools(cfg)
		if err == nil || !strings.Contains(err.Error(), "qemu-img") {
			t.Fatalf("err = %v, want qemu-img error", err)
		}
	})

	t.Run("extra disk auto requires qemu-img", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		cfg := &Config{ExtraDisks: []ExtraDisk{{Path: "/tmp/config.iso", Format: diskFormatAuto}}}
		err := CheckImageTools(cfg)
		if err == nil || !strings.Contains(err.Error(), "qemu-img") {
			t.Fatalf("err = %v, want qemu-img error", err)
		}
	})

	t.Run("explicit disk formats need no qemu-img", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		cfg := &Config{
			Source:     Source{Type: sourceDisk, Path: "/tmp/disk"},
			Disk:       Disk{Format: diskFormatRaw},
			ExtraDisks: []ExtraDisk{{Path: "/tmp/config.iso", Format: diskFormatRaw}},
		}
		if err := CheckImageTools(cfg); err != nil {
			t.Fatalf("CheckImageTools: %v", err)
		}
	})
}

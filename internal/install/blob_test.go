package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vmlite/internal/plan"
)

func testSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestBlobStep(t *testing.T) {
	data := []byte("binary-payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "tool")
	step := BlobStep{
		Blob: Blob{
			URL:    server.URL + "/tool",
			SHA256: testSHA(data),
			Dest:   dest,
			Mode:   0o750,
		},
	}
	ctx := context.Background()
	runner := &plan.LocalRunner{}

	ok, err := step.Check(ctx, runner)
	if err != nil || ok {
		t.Fatalf("Check before download = %v, %v; want false, nil", ok, err)
	}
	if err := step.Apply(ctx, runner); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read installed blob: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("blob = %q, want %q", got, data)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat blob: %v", err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("mode = %o, want 750", info.Mode().Perm())
	}
	ok, err = step.Check(ctx, runner)
	if err != nil || !ok {
		t.Fatalf("Check after download = %v, %v; want true, nil", ok, err)
	}
}

func TestBlobStepChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("wrong"))
	}))
	defer server.Close()

	step := BlobStep{Blob: Blob{
		URL:    server.URL + "/tool",
		SHA256: strings.Repeat("0", 64),
		Dest:   filepath.Join(t.TempDir(), "tool"),
	}}
	err := step.Apply(context.Background(), &plan.LocalRunner{})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if _, err := os.Stat(step.Blob.Dest); !os.IsNotExist(err) {
		t.Fatalf("failed download left a destination file: %v", err)
	}
}

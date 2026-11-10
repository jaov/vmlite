package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vmlite/internal/plan"
)

type Blob struct {
	URL    string
	SHA256 string
	Dest   string
	Mode   os.FileMode
}

type BlobStep struct {
	Blob        Blob
	Description string
}

func (s BlobStep) Describe() string {
	if s.Description != "" {
		return s.Description
	}
	return "download " + s.Blob.URL
}

func (s BlobStep) Check(ctx context.Context, r plan.Runner) (bool, error) {
	info, err := os.Stat(s.Blob.Dest)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("%q is a directory", s.Blob.Dest)
	}
	return verifyFileSHA(s.Blob.Dest, s.Blob.SHA256) == nil, nil
}

func (s BlobStep) Apply(ctx context.Context, r plan.Runner) error {
	if dry, ok := r.(interface{ IsDryRun() bool }); ok && dry.IsDryRun() {
		fmt.Printf("+ download %s -> %s\n", s.Blob.URL, s.Blob.Dest)
		return nil
	}
	return downloadBlob(s.Blob)
}

func validSHA(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func verifyFileSHA(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, want)
	}
	return nil
}

func downloadBlob(blob Blob) error {
	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Get(blob.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", blob.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected HTTP status %s", blob.URL, resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(blob.Dest), 0o755); err != nil {
		return fmt.Errorf("create download directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(blob.Dest), ".blob-*")
	if err != nil {
		return fmt.Errorf("create download temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("download %s: %w", blob.URL, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close downloaded blob: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, blob.SHA256) {
		return fmt.Errorf("%s sha256 mismatch: got %s, want %s", blob.URL, got, blob.SHA256)
	}

	mode := blob.Mode
	if mode == 0 {
		mode = 0o755
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod downloaded blob: %w", err)
	}
	if err := os.Rename(tmpName, blob.Dest); err != nil {
		return fmt.Errorf("install downloaded blob %q: %w", blob.Dest, err)
	}
	return nil
}

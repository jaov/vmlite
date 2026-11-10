package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vmlite/internal/plan"
)

func TestBuildPlanBinaryURLRequiresSHA(t *testing.T) {
	_, err := BuildPlan(Platform{Family: "debian"}, Options{
		BinaryURL: "https://example.invalid/vmlite",
		Prefix:    t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "binary-sha256") {
		t.Fatalf("error = %v, want sha requirement", err)
	}
}

func TestBuildPlanRejectsBothBinarySources(t *testing.T) {
	_, err := BuildPlan(Platform{Family: "debian"}, Options{
		BinaryPath: "/bin/true",
		BinaryURL:  "https://example.invalid/vmlite",
		Prefix:     t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %v, want mutual exclusion", err)
	}
}

func TestBuildPlanLocalBinary(t *testing.T) {
	destDir := t.TempDir()
	steps, err := BuildPlan(Platform{Family: "debian"}, Options{
		NoDeps:     true,
		BinaryPath: "/bin/true",
		Prefix:     destDir,
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("empty plan")
	}
	last := steps[len(steps)-1]
	if !strings.Contains(last.Describe(), "install vmlite") {
		t.Fatalf("last step = %q", last.Describe())
	}

	ctx := context.Background()
	runner := &plan.LocalRunner{}
	ok, err := last.Check(ctx, runner)
	if err != nil || ok {
		t.Fatalf("Check = %v, %v; want false, nil", ok, err)
	}
	if err := last.Apply(ctx, runner); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "vmlite")); err != nil {
		t.Fatalf("vmlite was not installed: %v", err)
	}
	ok, err = last.Check(ctx, runner)
	if err != nil || !ok {
		t.Fatalf("Check after install = %v, %v; want true, nil", ok, err)
	}
}

package install

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"vmlite/internal/plan"
)

func TestPackageStepDryRun(t *testing.T) {
	tests := []struct {
		family string
		want   string
	}{
		{"debian", "apt-get install -y pkg"},
		{"fedora", "dnf install -y pkg"},
		{"arch", "pacman -Sy --needed --noconfirm pkg"},
		{"suse", "zypper --non-interactive install pkg"},
	}

	for _, tc := range tests {
		t.Run(tc.family, func(t *testing.T) {
			var out bytes.Buffer
			runner := &plan.LocalRunner{
				DryRun:  true,
				Stdout:  &out,
				Geteuid: func() int { return 1000 },
			}
			manager := &Manager{Family: tc.family}
			steps := plan.Plan{manager.PackageStep([]string{"pkg"})}
			if err := steps.Run(context.Background(), runner, &out); err != nil {
				t.Fatalf("Plan.Run: %v\n%s", err, out.String())
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, out.String())
			}
		})
	}
}

type outputRunner struct {
	outputs map[string]string
}

func (r *outputRunner) Run(ctx context.Context, cmd plan.Command) error { return nil }

func (r *outputRunner) Output(ctx context.Context, cmd plan.Command) (string, error) {
	if out, ok := r.outputs[cmd.String()]; ok {
		return out, nil
	}
	return "", context.Canceled
}

func TestManagerInstalled(t *testing.T) {
	manager := &Manager{Family: "debian"}
	cmd := plan.Command{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}", "pkg"}}
	runner := &outputRunner{outputs: map[string]string{cmd.String(): "install ok installed"}}
	ok, err := manager.Installed(context.Background(), runner, "pkg")
	if err != nil {
		t.Fatalf("Installed: %v", err)
	}
	if !ok {
		t.Fatal("Installed = false, want true")
	}

	runner.outputs[cmd.String()] = "deinstall ok config-files"
	ok, err = manager.Installed(context.Background(), runner, "pkg")
	if err != nil || ok {
		t.Fatalf("Installed = %v, %v; want false, nil", ok, err)
	}
}

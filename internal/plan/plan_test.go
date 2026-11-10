package plan

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	ran      []Command
	outputs  map[string]string
	failures map[string]error
}

func (f *fakeRunner) Run(ctx context.Context, cmd Command) error {
	f.ran = append(f.ran, cmd)
	if f.failures != nil {
		return f.failures[cmd.String()]
	}
	return nil
}

func (f *fakeRunner) Output(ctx context.Context, cmd Command) (string, error) {
	f.ran = append(f.ran, cmd)
	if f.failures != nil {
		if err := f.failures[cmd.String()]; err != nil {
			return "", err
		}
	}
	if f.outputs != nil {
		return f.outputs[cmd.String()], nil
	}
	return "", nil
}

func TestPlanRunsUnsatisfiedStepsAndSkipsSatisfied(t *testing.T) {
	var events []string
	steps := Plan{
		StepFunc{
			Desc: "install qemu",
			CheckFn: func(context.Context, Runner) (bool, error) {
				return false, nil
			},
			ApplyFn: func(context.Context, Runner) error {
				events = append(events, "qemu")
				return nil
			},
		},
		StepFunc{
			Desc: "install libvirt",
			CheckFn: func(context.Context, Runner) (bool, error) {
				events = append(events, "check-libvirt")
				return true, nil
			},
			ApplyFn: func(context.Context, Runner) error {
				events = append(events, "libvirt")
				return nil
			},
		},
		StepFunc{
			Desc: "verify",
			ApplyFn: func(context.Context, Runner) error {
				events = append(events, "verify")
				return nil
			},
		},
	}

	var out bytes.Buffer
	if err := steps.Run(context.Background(), &fakeRunner{}, &out); err != nil {
		t.Fatalf("Plan.Run: %v", err)
	}
	if got, want := strings.Join(events, ","), "qemu,check-libvirt,verify"; got != want {
		t.Fatalf("events = %q, want %q", got, want)
	}
	if !strings.Contains(out.String(), "already satisfied") {
		t.Fatalf("log missing skip: %s", out.String())
	}
}

func TestPlanStopsOnError(t *testing.T) {
	boom := errors.New("boom")
	ran := false
	steps := Plan{
		StepFunc{
			Desc: "fail",
			ApplyFn: func(context.Context, Runner) error {
				return boom
			},
		},
		StepFunc{
			Desc: "must not run",
			ApplyFn: func(context.Context, Runner) error {
				ran = true
				return nil
			},
		},
	}
	err := steps.Run(context.Background(), &fakeRunner{}, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if ran {
		t.Fatal("plan continued after an error")
	}
}

func TestPlanCheckError(t *testing.T) {
	boom := errors.New("check failed")
	steps := Plan{
		StepFunc{
			Desc: "check error",
			CheckFn: func(context.Context, Runner) (bool, error) {
				return false, boom
			},
		},
	}
	err := steps.Run(context.Background(), &fakeRunner{}, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want check error", err)
	}
}

func TestPlanRejectsNilStep(t *testing.T) {
	steps := Plan{nil}
	err := steps.Run(context.Background(), &fakeRunner{}, nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("err = %v, want nil-step error", err)
	}
}

func TestEnsure(t *testing.T) {
	step := Ensure("demo", nil, func(context.Context, Runner) error { return nil })
	if step.Describe() != "demo" {
		t.Fatalf("Describe = %q", step.Describe())
	}
	ok, err := step.Check(context.Background(), &fakeRunner{})
	if err != nil || ok {
		t.Fatalf("Check = %v, %v; want false, nil", ok, err)
	}
}

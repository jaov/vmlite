package plan

import (
	"context"
	"fmt"
	"io"
)

// Step is one idempotent action in a plan.
type Step interface {
	Describe() string
	// Check reports whether the step is already satisfied.
	Check(ctx context.Context, r Runner) (bool, error)
	Apply(ctx context.Context, r Runner) error
}

// StepFunc adapts functions into a Step.
type StepFunc struct {
	Desc    string
	CheckFn func(ctx context.Context, r Runner) (bool, error)
	ApplyFn func(ctx context.Context, r Runner) error
}

func (s StepFunc) Describe() string { return s.Desc }

func (s StepFunc) Check(ctx context.Context, r Runner) (bool, error) {
	if s.CheckFn == nil {
		return false, nil
	}
	return s.CheckFn(ctx, r)
}

func (s StepFunc) Apply(ctx context.Context, r Runner) error {
	if s.ApplyFn == nil {
		return nil
	}
	return s.ApplyFn(ctx, r)
}

// Ensure is a convenience constructor for a Step.
func Ensure(desc string, check func(context.Context, Runner) (bool, error), apply func(context.Context, Runner) error) Step {
	return StepFunc{Desc: desc, CheckFn: check, ApplyFn: apply}
}

// Plan is an ordered composition of steps.
type Plan []Step

// Run executes the plan, skipping satisfied steps and stopping at the first
// error.
func (p Plan) Run(ctx context.Context, r Runner, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	for i, step := range p {
		if step == nil {
			return fmt.Errorf("plan step %d is nil", i)
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		fmt.Fprintf(out, "==> %s\n", step.Describe())
		ok, err := step.Check(ctx, r)
		if err != nil {
			return fmt.Errorf("%s: check: %w", step.Describe(), err)
		}
		if ok {
			fmt.Fprintln(out, "    already satisfied")
			continue
		}
		if err := step.Apply(ctx, r); err != nil {
			return fmt.Errorf("%s: %w", step.Describe(), err)
		}
	}
	return nil
}

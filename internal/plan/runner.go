package plan

import "context"

// Runner executes commands locally or remotely.
type Runner interface {
	Run(ctx context.Context, cmd Command) error
	Output(ctx context.Context, cmd Command) (string, error)
}

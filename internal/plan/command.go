package plan

import (
	"io"
	"strconv"
	"strings"
)

// Command describes one process invocation.
type Command struct {
	Name  string
	Args  []string
	Env   []string
	Stdin io.Reader
	Sudo  bool
}

// effective returns the command to execute. When sudo is requested and the
// current process is not root, environment assignments are moved behind
// `sudo env` so they actually reach the child.
func (c Command) effective(root bool) (string, []string, []string) {
	if !c.Sudo || root {
		return c.Name, c.Args, c.Env
	}

	inner := append([]string{c.Name}, c.Args...)
	if len(c.Env) > 0 {
		inner = append([]string{"env"}, append(append([]string{}, c.Env...), inner...)...)
	}
	return "sudo", inner, nil
}

// String renders the effective command for logs and dry runs.
func (c Command) String() string {
	return c.effectiveString(false)
}

func (c Command) effectiveString(root bool) string {
	name, args, env := c.effective(root)
	parts := make([]string, 0, len(env)+1+len(args))
	if len(env) > 0 {
		parts = append(parts, "env")
		parts = append(parts, env...)
	}
	parts = append(parts, name)
	parts = append(parts, args...)

	for i, part := range parts {
		if part == "" || strings.ContainsAny(part, " \t\n\"'") {
			parts[i] = strconv.Quote(part)
		}
	}
	return strings.Join(parts, " ")
}

package install

import (
	"context"
	"fmt"
	"strings"

	"vmlite/internal/plan"
)

// Manager installs packages for one distribution family.
type Manager struct {
	Family string
}

func NewManager(p Platform) (*Manager, error) {
	switch p.Family {
	case "debian", "fedora", "arch", "suse":
		return &Manager{Family: p.Family}, nil
	default:
		return nil, fmt.Errorf("unsupported package family %q", p.Family)
	}
}

func (m *Manager) Installed(ctx context.Context, r plan.Runner, pkg string) (bool, error) {
	var cmd plan.Command
	switch m.Family {
	case "debian":
		cmd = plan.Command{Name: "dpkg-query", Args: []string{"-W", "-f=${Status}", pkg}}
	case "fedora", "suse":
		cmd = plan.Command{Name: "rpm", Args: []string{"-q", pkg}}
	case "arch":
		cmd = plan.Command{Name: "pacman", Args: []string{"-Q", pkg}}
	default:
		return false, fmt.Errorf("unsupported package family %q", m.Family)
	}

	out, err := r.Output(ctx, cmd)
	if err != nil {
		return false, nil
	}
	if m.Family == "debian" {
		return strings.Contains(out, "install ok installed"), nil
	}
	return strings.TrimSpace(out) != "", nil
}

func (m *Manager) Update(ctx context.Context, r plan.Runner) error {
	switch m.Family {
	case "debian":
		return r.Run(ctx, plan.Command{Name: "apt-get", Args: []string{"update"}, Sudo: true})
	default:
		return nil
	}
}

func (m *Manager) Install(ctx context.Context, r plan.Runner, packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	switch m.Family {
	case "debian":
		return r.Run(ctx, plan.Command{
			Name: "apt-get",
			Args: append([]string{"install", "-y"}, packages...),
			Env:  []string{"DEBIAN_FRONTEND=noninteractive"},
			Sudo: true,
		})
	case "fedora":
		return r.Run(ctx, plan.Command{
			Name: "dnf",
			Args: append([]string{"install", "-y"}, packages...),
			Sudo: true,
		})
	case "arch":
		return r.Run(ctx, plan.Command{
			Name: "pacman",
			Args: append([]string{"-Sy", "--needed", "--noconfirm"}, packages...),
			Sudo: true,
		})
	case "suse":
		return r.Run(ctx, plan.Command{
			Name: "zypper",
			Args: append([]string{"--non-interactive", "install"}, packages...),
			Sudo: true,
		})
	default:
		return fmt.Errorf("unsupported package family %q", m.Family)
	}
}

type packageStep struct {
	manager  *Manager
	packages []string
}

func (s packageStep) Describe() string {
	return fmt.Sprintf("install host packages (%s)", strings.Join(s.packages, ", "))
}

func (s packageStep) Check(ctx context.Context, r plan.Runner) (bool, error) {
	for _, pkg := range s.packages {
		ok, err := s.manager.Installed(ctx, r, pkg)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func (s packageStep) Apply(ctx context.Context, r plan.Runner) error {
	if err := s.manager.Update(ctx, r); err != nil {
		return err
	}
	return s.manager.Install(ctx, r, s.packages...)
}

// PackageStep ensures a list of packages is installed.
func (m *Manager) PackageStep(packages []string) plan.Step {
	return packageStep{manager: m, packages: packages}
}

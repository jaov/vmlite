package guest

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"vmlite/internal/config"
	"vmlite/internal/host"
	"vmlite/internal/libvirt"
	"vmlite/internal/plan"
	"vmlite/internal/provision"
	"vmlite/internal/state"
)

type Target struct {
	User string
	Key  string
	IP   string
}

func ResolveTarget(configPath, stateDir, user string, timeout time.Duration) (*Target, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	layout, err := state.NewProjectLayout(configPath, stateDir, cfg.Name)
	if err != nil {
		return nil, err
	}
	st, err := state.Load(layout.StateFile)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("no state for %q at %s", cfg.Name, layout.StateFile)
	}
	if st.LibvirtURI == host.SessionURI {
		return nil, fmt.Errorf("SSH access requires qemu:///system with a guest-visible network")
	}
	text, exists := libvirt.Status(st.LibvirtURI, st.Name)
	if !exists {
		return nil, fmt.Errorf("VM %q is missing", st.Name)
	}
	if !strings.Contains(text, "running") {
		return nil, fmt.Errorf("VM %q is not running; run vmlite up or vmlite start", st.Name)
	}
	if st.SSHKeyPath == "" {
		return nil, fmt.Errorf("VM %q has no managed SSH key; add ssh.key or enable provisioners and recreate", st.Name)
	}

	ip := st.IP
	if timeout > 0 {
		ip, err = provision.WaitForIPv4(st.LibvirtURI, st.Name, timeout)
		if err != nil {
			return nil, err
		}
		if st.IP != ip {
			st.IP = ip
			if err := state.Save(layout.StateFile, st); err != nil {
				return nil, err
			}
		}
	}
	if ip == "" {
		return nil, fmt.Errorf("VM %q has no IP; run vmlite provision or wait for DHCP", st.Name)
	}
	if user == "" {
		user = st.SSHUser
	}
	if user == "" {
		user = cfg.SSHUser()
	}
	return &Target{User: user, Key: st.SSHKeyPath, IP: ip}, nil
}

func WaitForSSH(target *Target, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	runner := &plan.SSHRunner{
		Host:   target.IP,
		User:   target.User,
		Key:    target.Key,
		Stdout: io.Discard,
		Stderr: io.Discard,
	}
	return provision.WaitForSSH(context.Background(), runner, timeout)
}

func SSHOptionArgs(target *Target, tty bool) []string {
	args := []string{
		"-i", target.Key,
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-o", "LogLevel=ERROR",
	}
	if tty {
		args = append(args, "-t")
	}
	return args
}

func StdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

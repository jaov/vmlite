package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"vmlite/internal/plan"
)

type guestTarget struct {
	cfg  *Config
	st   *State
	user string
	key  string
	ip   string
}

func resolveGuestTarget(configPath, stateDir, user string, timeout time.Duration) (*guestTarget, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	layout, err := newProjectLayout(configPath, stateDir, cfg.Name)
	if err != nil {
		return nil, err
	}
	st, err := loadState(layout.StateFile)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("no state for %q at %s", cfg.Name, layout.StateFile)
	}
	if st.LibvirtURI == sessionURI {
		return nil, fmt.Errorf("SSH access requires qemu:///system with a guest-visible network")
	}
	text, exists := domainStatus(st.LibvirtURI, st.Name)
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
		ip, err = waitForIPv4(st.LibvirtURI, st.Name, timeout)
		if err != nil {
			return nil, err
		}
		if st.IP != ip {
			st.IP = ip
			if err := saveState(layout.StateFile, st); err != nil {
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
		user = cfg.sshUser()
	}
	return &guestTarget{cfg: cfg, st: st, user: user, key: st.SSHKeyPath, ip: ip}, nil
}

func waitForGuestSSH(target *guestTarget, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	runner := &plan.SSHRunner{
		Host:   target.ip,
		User:   target.user,
		Key:    target.key,
		Stdout: io.Discard,
		Stderr: io.Discard,
	}
	return waitForSSH(context.Background(), runner, timeout)
}

func sshOptionArgs(target *guestTarget, tty bool) []string {
	args := []string{
		"-i", target.key,
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

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	guestinstall "vmlite/internal/installer"
	"vmlite/internal/plan"
)

func newStateFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parseConfigArgs(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", errUsage
	}
	rest := fs.Args()
	if len(rest) > 1 {
		return "", errUsage
	}
	if len(rest) == 1 {
		return rest[0], nil
	}
	return "vm.json", nil
}

func derivePublicKey(privatePath string) (string, error) {
	pubPath := privatePath + ".pub"
	if data, err := os.ReadFile(pubPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		return "", fmt.Errorf("ssh-keygen is required to derive the public key: %w", err)
	}
	out, err := exec.Command("ssh-keygen", "-y", "-f", privatePath).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("derive public key from %q: %w: %s", privatePath, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func ensureSSHKey(cfg *Config, layout projectLayout) error {
	if !cfg.needsSSH() {
		return nil
	}

	if strings.TrimSpace(cfg.SSH.Key) != "" {
		key, err := filepath.Abs(cfg.SSH.Key)
		if err != nil {
			return fmt.Errorf("resolve ssh key %q: %w", cfg.SSH.Key, err)
		}
		pub, err := derivePublicKey(key)
		if err != nil {
			return err
		}
		cfg.SSH.Key = key
		cfg.sshPublicKey = pub
		cfg.sshKeyGenerated = false
		return nil
	}

	if !cfg.canGenerateSSHKey() {
		return fmt.Errorf("ssh.key is required for source.type %q", cfg.Source.Type)
	}

	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		return fmt.Errorf("ssh-keygen is required for provisioners: %w", err)
	}
	if err := os.MkdirAll(layout.AssetDir, 0o755); err != nil {
		return fmt.Errorf("create asset dir: %w", err)
	}
	key := filepath.Join(layout.AssetDir, "id_ed25519")
	if _, err := os.Stat(key); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("check ssh key: %w", err)
		}
		cmd := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("generate ssh key: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	pub, err := derivePublicKey(key)
	if err != nil {
		return err
	}
	cfg.SSH.Key = key
	cfg.sshPublicKey = pub
	cfg.sshKeyGenerated = true
	return nil
}

func waitForIPv4(uri, name string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		for _, source := range []string{"arp", "lease"} {
			out, err := runVirsh(uri, "domifaddr", name, "--source", source)
			last = out
			if err != nil {
				continue
			}
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "ipv4" && i+1 < len(fields) {
						return strings.Split(fields[i+1], "/")[0], nil
					}
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("no guest IPv4 lease for %q within %s:\n%s", name, timeout, last)
}

func waitForSSH(ctx context.Context, runner *plan.SSHRunner, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		attempt, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := runner.Run(attempt, plan.Command{Name: "true"})
		cancel()
		if err == nil {
			return nil
		}
		last = err
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("SSH to %s@%s did not become ready within %s: %w", runner.User, runner.Host, timeout, last)
}

type provisionUpload struct {
	source string
	target string
	data   []byte
}

type shellStep struct {
	desc      string
	id        string
	typ       string
	script    []byte
	hash      string
	run       string
	state     *State
	stateFile string
	runner    *plan.SSHRunner
	uploads   []provisionUpload
	env       map[string]string
	force     bool
	dryRun    bool
}

func (s *shellStep) Describe() string { return s.desc }

func (s *shellStep) Check(ctx context.Context, r plan.Runner) (bool, error) {
	if s.dryRun || s.force || s.run == "always" {
		return false, nil
	}
	for _, rec := range s.state.Provisioned {
		if rec.ID == s.id && rec.SHA256 == s.hash {
			return true, nil
		}
	}
	return false, nil
}

func (s *shellStep) Apply(ctx context.Context, r plan.Runner) error {
	script := s.script
	var env []string
	if s.typ == "powershell" {
		script = powershellScriptWithEnv(script, s.env)
	} else {
		env = envSlice(s.env)
	}
	if len(s.uploads) > 0 {
		if err := s.uploadFiles(); err != nil {
			return err
		}
	}

	name := "bash"
	args := []string{"-s"}
	sudo := s.runner.User != "root"
	if s.typ == "powershell" {
		name = "powershell"
		args = []string{"-NoProfile", "-NonInteractive", "-Command", "-"}
		sudo = false
	}
	if err := s.runner.Run(ctx, plan.Command{
		Name:  name,
		Args:  args,
		Sudo:  sudo,
		Env:   env,
		Stdin: bytes.NewReader(script),
	}); err != nil {
		return err
	}
	if s.dryRun {
		return nil
	}

	var records []ProvisionRecord
	for _, rec := range s.state.Provisioned {
		if rec.ID != s.id {
			records = append(records, rec)
		}
	}
	records = append(records, ProvisionRecord{
		ID:          s.id,
		SHA256:      s.hash,
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
	})
	s.state.Provisioned = records
	s.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return saveState(s.stateFile, s.state)
}

func guestSSHRunner(cfg *Config, st *State, ip string, dryRun bool) *plan.SSHRunner {
	user := st.SSHUser
	if user == "" {
		user = cfg.sshUser()
	}
	return &plan.SSHRunner{
		Host:   ip,
		User:   user,
		Key:    st.SSHKeyPath,
		DryRun: dryRun,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// finishInstaller waits for an unattended installer to reboot, ejects the
// installer CD, and starts the installed disk. It also accepts an already
// running installed system if SSH is reachable.
func finishInstaller(cfg *Config, st *State, layout projectLayout, dryRun bool) error {
	if st.InstallPhase == installPhaseInstalled {
		return nil
	}
	if st.InstallerCompletion == string(guestinstall.CompletionSSH) {
		if dryRun {
			fmt.Println("Would wait for the installer to expose SSH")
		}
		return nil
	}
	if dryRun {
		fmt.Println("Would wait for the installer, eject its media, and boot the installed disk")
		return nil
	}

	if strings.TrimSpace(st.SSHKeyPath) != "" {
		if ip, err := waitForIPv4(st.LibvirtURI, st.Name, 15*time.Second); err == nil {
			runner := guestSSHRunner(cfg, st, ip, false)
			attempt, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := runner.Run(attempt, plan.Command{Name: "true"})
			cancel()
			if err == nil {
				if err := ejectInstallerMedia(st.LibvirtURI, st.Name, true); err != nil {
					return err
				}
				if err := clearInstallerBoot(st.LibvirtURI, st.Name); err != nil {
					return err
				}
				return markInstalled(st, layout)
			}
		}
	}

	timeout := 30 * time.Minute
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		text, exists := domainStatus(st.LibvirtURI, st.Name)
		if !exists {
			return fmt.Errorf("installer VM %q disappeared", st.Name)
		}
		text = strings.TrimSpace(text)
		if strings.Contains(text, "shut off") {
			if err := ejectInstallerMedia(st.LibvirtURI, st.Name, false); err != nil {
				return err
			}
			if err := clearInstallerBoot(st.LibvirtURI, st.Name); err != nil {
				return err
			}
			if out, err := runVirsh(st.LibvirtURI, "start", st.Name); err != nil {
				return fmt.Errorf("start installed VM %q: %w: %s", st.Name, err, strings.TrimSpace(out))
			}
			return markInstalled(st, layout)
		}
		if !strings.Contains(text, "running") && !strings.Contains(text, "in shutdown") {
			return fmt.Errorf("installer VM %q is %q; expected running or shut off", st.Name, text)
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("installer VM %q did not finish within %s", st.Name, timeout)
}

func markInstalled(st *State, layout projectLayout) error {
	st.InstallPhase = installPhaseInstalled
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return saveState(layout.StateFile, st)
}

func ejectInstallerMedia(uri, name string, live bool) error {
	out, err := runVirsh(uri, "domblklist", name, "--details")
	if err != nil {
		return fmt.Errorf("list media for %q: %w: %s", name, err, strings.TrimSpace(out))
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[1] != "cdrom" {
			continue
		}
		if fields[3] == "-" {
			continue
		}
		target := fields[2]
		args := []string{"change-media", name, target, "--eject"}
		if live {
			args = append(args, "--live")
		}
		args = append(args, "--config")
		if out, err := runVirsh(uri, args...); err != nil {
			if strings.Contains(out, "doesn't have media") {
				continue
			}
			return fmt.Errorf("eject %s from %q: %w: %s", target, name, err, strings.TrimSpace(out))
		}
	}
	return nil
}

var installerKernelXML = regexp.MustCompile(`(?s)\s*<kernel\b[^>]*(?:/>|>.*?</kernel>)`)
var installerInitrdXML = regexp.MustCompile(`(?s)\s*<initrd\b[^>]*(?:/>|>.*?</initrd>)`)
var installerCmdlineXML = regexp.MustCompile(`(?s)\s*<cmdline\b[^>]*>.*?</cmdline>`)
var installerOnRebootXML = regexp.MustCompile(`(?s)\s*<on_reboot>.*?</on_reboot>`)

// clearInstallerBoot removes the direct kernel/initrd boot described by
// virt-install's --location install phase. Without this the defined domain
// would boot the installer kernel again after the installer reboots.
func clearInstallerBoot(uri, name string) error {
	out, err := runVirsh(uri, "dumpxml", name)
	if err != nil {
		return fmt.Errorf("dump domain %q: %w: %s", name, err, strings.TrimSpace(out))
	}
	cleaned := out
	for _, re := range []*regexp.Regexp{
		installerKernelXML,
		installerInitrdXML,
		installerCmdlineXML,
		installerOnRebootXML,
	} {
		cleaned = re.ReplaceAllString(cleaned, "")
	}
	if cleaned == out {
		return nil
	}

	tmp, err := os.CreateTemp("", "vmlite-domain-*.xml")
	if err != nil {
		return fmt.Errorf("create domain XML temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(cleaned); err != nil {
		tmp.Close()
		return fmt.Errorf("write domain XML: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close domain XML: %w", err)
	}
	if out, err := runVirsh(uri, "define", tmpName); err != nil {
		return fmt.Errorf("redefine installed VM %q: %w: %s", name, err, strings.TrimSpace(out))
	}
	return nil
}

func provisionTimeouts(st *State, override time.Duration) (time.Duration, time.Duration) {
	ipTimeout := 5 * time.Minute
	sshTimeout := 5 * time.Minute
	if st.InstallerType != "" {
		sshTimeout = 10 * time.Minute
	}
	if st.InstallerCompletion == string(guestinstall.CompletionSSH) {
		ipTimeout = 30 * time.Minute
		sshTimeout = 30 * time.Minute
	}
	if override > 0 {
		ipTimeout = override
		sshTimeout = override
	}
	return ipTimeout, sshTimeout
}

func (s *shellStep) uploadFiles() error {
	for _, upload := range s.uploads {
		args := []string{
			"-i", s.runner.Key,
			"-o", "BatchMode=yes",
			"-o", "IdentitiesOnly=yes",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "ConnectTimeout=10",
			"-o", "LogLevel=ERROR",
			upload.source,
			s.runner.User + "@" + s.runner.Host + ":" + upload.target,
		}
		if s.dryRun {
			fmt.Printf("+ scp %s\n", strings.Join(args, " "))
			continue
		}
		cmd := exec.Command("scp", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("upload %s to %s@%s:%s: %w", upload.source, s.runner.User, s.runner.Host, upload.target, err)
		}
	}
	return nil
}

func envSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+env[key])
	}
	return result
}

func powershellScriptWithEnv(script []byte, env map[string]string) []byte {
	if len(env) == 0 {
		return script
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out bytes.Buffer
	for _, key := range keys {
		value := strings.ReplaceAll(env[key], "'", "''")
		fmt.Fprintf(&out, "$env:%s = '%s'\n", key, value)
	}
	out.Write(script)
	return out.Bytes()
}

func provisionHash(script []byte, env map[string]string, uploads []provisionUpload) string {
	h := sha256.New()
	_, _ = h.Write(script)
	for _, entry := range envSlice(env) {
		_, _ = h.Write([]byte("env\x00" + entry + "\x00"))
	}
	for _, upload := range uploads {
		_, _ = h.Write([]byte("upload\x00" + upload.source + "\x00" + upload.target + "\x00"))
		_, _ = h.Write(upload.data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func readProvisionUploads(p Provisioner, layout projectLayout) ([]provisionUpload, error) {
	uploads := make([]provisionUpload, 0, len(p.Uploads))
	for _, upload := range p.Uploads {
		source := upload.Source
		if !filepath.IsAbs(source) {
			source = filepath.Join(filepath.Dir(layout.ConfigPath), source)
		}
		source, err := filepath.Abs(source)
		if err != nil {
			return nil, fmt.Errorf("resolve upload source %q: %w", upload.Source, err)
		}
		info, err := os.Stat(source)
		if err != nil {
			return nil, fmt.Errorf("upload source %q is not accessible: %w", upload.Source, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("upload source %q is a directory; uploads currently support files only", upload.Source)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read upload source %q: %w", upload.Source, err)
		}
		uploads = append(uploads, provisionUpload{source: source, target: upload.Target, data: data})
	}
	return uploads, nil
}

func provisionVM(cfg *Config, st *State, layout projectLayout, force, dryRun bool, timeout time.Duration) error {
	if len(cfg.Provisioners) == 0 && st.InstallerType == "" {
		return nil
	}
	completionSSH := st.InstallerCompletion == string(guestinstall.CompletionSSH)
	if len(cfg.Provisioners) > 0 {
		if strings.TrimSpace(st.SSHKeyPath) == "" {
			return fmt.Errorf("VM %q has no managed SSH key; recreate it with provisioners enabled", st.Name)
		}
		if st.LibvirtURI == sessionURI {
			return fmt.Errorf("shell provisioners require qemu:///system with a guest-visible network")
		}
		if cfg.Network.Mode == networkModeUser {
			return fmt.Errorf("shell provisioners require a guest-visible network, not network.mode=user")
		}
	}
	if text, exists := domainStatus(st.LibvirtURI, st.Name); !exists {
		return fmt.Errorf("VM %q is missing", st.Name)
	} else if st.InstallerType == "" && !strings.Contains(text, "running") {
		return fmt.Errorf("VM %q is not running", st.Name)
	} else if st.InstallerType != "" && completionSSH && strings.Contains(text, "shut off") {
		return fmt.Errorf("VM %q is shut off; Windows install did not reboot into the installed OS", st.Name)
	} else if st.InstallerType != "" && !strings.Contains(text, "running") && !strings.Contains(text, "shut off") && !strings.Contains(text, "in shutdown") {
		return fmt.Errorf("VM %q is %q; expected running or shut off", st.Name, strings.TrimSpace(text))
	}

	if st.InstallerType != "" {
		if err := finishInstaller(cfg, st, layout, dryRun); err != nil {
			return err
		}
		if len(cfg.Provisioners) == 0 && !completionSSH {
			return nil
		}
	}

	var ip string
	if dryRun {
		ip = st.IP
		if ip == "" {
			ip = "<guest-ip>"
		}
	} else {
		ipTimeout, _ := provisionTimeouts(st, timeout)
		var err error
		ip, err = waitForIPv4(st.LibvirtURI, st.Name, ipTimeout)
		if err != nil {
			return err
		}
		st.IP = ip
		if err := saveState(layout.StateFile, st); err != nil {
			return err
		}
	}

	runner := guestSSHRunner(cfg, st, ip, dryRun)
	if !dryRun {
		_, sshTimeout := provisionTimeouts(st, timeout)
		if err := waitForSSH(context.Background(), runner, sshTimeout); err != nil {
			return err
		}
		if completionSSH && st.InstallPhase != installPhaseInstalled {
			if err := markInstalled(st, layout); err != nil {
				return err
			}
		}
	}
	if len(cfg.Provisioners) == 0 {
		return nil
	}

	var steps plan.Plan
	for i, p := range cfg.Provisioners {
		var script []byte
		var id string
		if strings.TrimSpace(p.Path) != "" {
			path := p.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(filepath.Dir(layout.ConfigPath), path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read provisioner %q: %w", p.Path, err)
			}
			script = data
			id = path
		} else {
			script = []byte(p.Inline)
			id = fmt.Sprintf("inline-%d", i)
		}
		uploads, err := readProvisionUploads(p, layout)
		if err != nil {
			return err
		}
		hash := provisionHash(script, p.Env, uploads)
		run := p.Run
		if run == "" {
			run = "once"
		}
		typ := p.Type
		if typ == "" {
			typ = "shell"
		}
		steps = append(steps, &shellStep{
			desc:      fmt.Sprintf("%s provisioner %s", typ, id),
			id:        id,
			typ:       typ,
			script:    script,
			hash:      hash,
			run:       run,
			state:     st,
			stateFile: layout.StateFile,
			runner:    runner,
			uploads:   uploads,
			env:       p.Env,
			force:     force,
			dryRun:    dryRun,
		})
	}

	return steps.Run(context.Background(), runner, os.Stdout)
}

func provisionFlagSet() (*flag.FlagSet, *string, *bool, *bool, *time.Duration) {
	fs := newStateFlagSet("provision")
	stateDir := fs.String("state-dir", "", "state directory (default: .vmlite next to the config)")
	force := fs.Bool("force", false, "rerun once-provisioners")
	dryRun := fs.Bool("dry-run", false, "print actions without changing anything")
	timeout := fs.Duration("timeout", 0, "maximum wait for guest readiness (default: 5m, or 30m for SSH-completing installers)")
	return fs, stateDir, force, dryRun, timeout
}

func runProvision(args []string) error {
	fs, stateDir, force, dryRun, timeout := provisionFlagSet()
	configPath, err := parseConfigArgs(fs, args)
	if err != nil {
		return err
	}
	if *timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	layout, err := newProjectLayout(configPath, *stateDir, cfg.Name)
	if err != nil {
		return err
	}
	lock, err := acquireProjectLock(layout)
	if err != nil {
		return err
	}
	defer lock.Close()
	st, err := loadState(layout.StateFile)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("no state for %q at %s", cfg.Name, layout.StateFile)
	}
	return provisionVM(cfg, st, layout, *force, *dryRun, *timeout)
}

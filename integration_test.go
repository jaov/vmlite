//go:build integration

package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var (
	vmliteBin   string
	testURI     string
	skipReason  string
	nameCounter atomic.Int64
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vmlite-integration-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration setup: %v\n", err)
		os.Exit(1)
	}

	code := runIntegrationSuite(m, dir)
	os.RemoveAll(dir)
	os.Exit(code)
}

func runIntegrationSuite(m *testing.M, dir string) int {
	bin := filepath.Join(dir, "vmlite")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "integration setup: build vmlite: %v\n%s", err, out)
		return 1
	}
	vmliteBin = bin

	skipReason = integrationSkipReason()
	return m.Run()
}

func integrationSkipReason() string {
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return "/dev/kvm is not available: " + err.Error()
	}
	if _, err := exec.LookPath("virt-install"); err != nil {
		return "virt-install not found in PATH"
	}
	if _, err := exec.LookPath("virsh"); err != nil {
		return "virsh not found in PATH"
	}

	testURI = os.Getenv("VMLITE_CONNECT")
	if testURI == "" {
		testURI = sessionURI
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "virsh", "-c", testURI, "nodeinfo").CombinedOutput(); err != nil {
		return fmt.Sprintf("libvirt at %s is unavailable: %v: %s", testURI, err, strings.TrimSpace(string(out)))
	}
	return ""
}

func requireIntegration(t *testing.T) {
	t.Helper()
	if skipReason != "" {
		t.Skip(skipReason)
	}
}

func uniqueName() string {
	return fmt.Sprintf("vmlite-it-%d-%d", os.Getpid(), nameCounter.Add(1))
}

func envWith(overrides map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		key := strings.SplitN(kv, "=", 2)[0]
		if _, ok := overrides[key]; ok {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}

func runVmlite(t *testing.T, uri, configPath string) (string, error) {
	t.Helper()
	return runVmliteArgs(t, []string{"--connect", uri, configPath}, nil)
}

func runVmliteArgs(t *testing.T, args []string, env map[string]string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, vmliteBin, args...)
	cmd.Env = envWith(env)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("vmlite timed out\n%s", out)
	}
	return string(out), err
}

func virsh(t *testing.T, uri string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	full := append([]string{"-c", uri}, args...)
	out, err := exec.CommandContext(ctx, "virsh", full...).CombinedOutput()
	return string(out), err
}

func noInstallConfig(name string, mounts []Mount) Config {
	return Config{
		Name:      name,
		Memory:    256,
		CPUs:      1,
		OSVariant: "debian12",
		Source:    Source{Type: sourceNone},
		Disk:      Disk{Size: 1},
		Network:   Network{Mode: networkModeAuto},
		Graphics:  Graphics{Type: graphicsNone},
		Mounts:    mounts,
	}
}

func writeConfig(t *testing.T, cfg Config) string {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vm.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// mountDir creates a host directory that libvirt-qemu can read in system mode.
func mountDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "vmlite-mount-")
	if err != nil {
		t.Fatalf("create mount dir: %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("chmod mount dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func registerCleanup(t *testing.T, uri, name string) {
	t.Helper()
	t.Cleanup(func() {
		if os.Getenv("VMLITE_TEST_KEEP") == "1" {
			t.Logf("VMLITE_TEST_KEEP=1: leaving domain %s in place", name)
			return
		}
		if _, err := virsh(t, uri, "dominfo", name); err != nil {
			return
		}
		_, _ = virsh(t, uri, "destroy", name)
		if out, err := virsh(t, uri, "undefine", name, "--remove-all-storage"); err != nil {
			t.Logf("cleanup: undefine %s: %v: %s", name, err, strings.TrimSpace(out))
			if _, err := virsh(t, uri, "undefine", name); err != nil {
				t.Logf("cleanup: domain %s may still exist", name)
			}
		}
	})
}

type domainXML struct {
	Memory struct {
		Unit  string `xml:"unit,attr"`
		Value int    `xml:",chardata"`
	} `xml:"memory"`
	VCPU struct {
		Value int `xml:",chardata"`
	} `xml:"vcpu"`
	Devices struct {
		Disks []struct {
			Source struct {
				File string `xml:"file,attr"`
			} `xml:"source"`
		} `xml:"disk"`
		Interfaces []struct {
			Type   string `xml:"type,attr"`
			Source struct {
				Network string `xml:"network,attr"`
			} `xml:"source"`
		} `xml:"interface"`
		Filesystems []struct {
			Type       string `xml:"type,attr"`
			AccessMode string `xml:"accessmode,attr"`
			Source     struct {
				Dir string `xml:"dir,attr"`
			} `xml:"source"`
			Target struct {
				Dir string `xml:"dir,attr"`
			} `xml:"target"`
		} `xml:"filesystem"`
	} `xml:"devices"`
}

func fetchDomain(t *testing.T, uri, name string) domainXML {
	t.Helper()
	out, err := virsh(t, uri, "dumpxml", name)
	if err != nil {
		t.Fatalf("dumpxml %s: %v\n%s", name, err, out)
	}

	var d domainXML
	if err := xml.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("parse dumpxml %s: %v", name, err)
	}
	return d
}

func memoryMiB(t *testing.T, d domainXML) int {
	t.Helper()
	switch d.Memory.Unit {
	case "", "KiB":
		return d.Memory.Value / 1024
	case "MiB":
		return d.Memory.Value
	case "GiB":
		return d.Memory.Value * 1024
	default:
		t.Fatalf("unexpected memory unit %q", d.Memory.Unit)
		return 0
	}
}

func createVM(t *testing.T, uri string, mounts []Mount) string {
	t.Helper()
	name := uniqueName()
	registerCleanup(t, uri, name)

	cfg := noInstallConfig(name, mounts)
	path := writeConfig(t, cfg)

	out, err := runVmlite(t, uri, path)
	if err != nil {
		t.Fatalf("vmlite create failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created successfully") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	return name
}

func verifyDomain(t *testing.T, uri, name string, mounts []Mount) {
	t.Helper()
	d := fetchDomain(t, uri, name)

	if got := memoryMiB(t, d); got != 256 {
		t.Errorf("memory = %d MiB, want 256", got)
	}
	if d.VCPU.Value != 1 {
		t.Errorf("vcpus = %d, want 1", d.VCPU.Value)
	}
	if len(d.Devices.Disks) != 1 {
		t.Fatalf("disks = %d, want 1", len(d.Devices.Disks))
	}
	if d.Devices.Disks[0].Source.File == "" {
		t.Error("disk has no source file")
	}

	switch uri {
	case sessionURI:
		if len(d.Devices.Interfaces) != 1 {
			t.Fatalf("interfaces = %d, want 1", len(d.Devices.Interfaces))
		}
		if got := d.Devices.Interfaces[0].Type; got != "user" {
			t.Errorf("interface type = %q, want user", got)
		}
	case systemURI:
		if len(d.Devices.Interfaces) != 1 {
			t.Fatalf("interfaces = %d, want 1", len(d.Devices.Interfaces))
		}
		if got := d.Devices.Interfaces[0].Type; got != "network" {
			t.Errorf("interface type = %q, want network", got)
		}
		if got := d.Devices.Interfaces[0].Source.Network; got != "default" {
			t.Errorf("network = %q, want default", got)
		}
	}

	if len(d.Devices.Filesystems) != len(mounts) {
		t.Fatalf("filesystems = %d, want %d", len(d.Devices.Filesystems), len(mounts))
	}
	for i, m := range mounts {
		fs := d.Devices.Filesystems[i]
		absSource, err := filepath.Abs(m.Source)
		if err != nil {
			t.Fatalf("abs %q: %v", m.Source, err)
		}
		if fs.Type != "mount" {
			t.Errorf("filesystem[%d] type = %q, want mount", i, fs.Type)
		}
		if fs.AccessMode != "mapped" {
			t.Errorf("filesystem[%d] accessmode = %q, want mapped", i, fs.AccessMode)
		}
		if fs.Source.Dir != absSource {
			t.Errorf("filesystem[%d] source = %q, want %q", i, fs.Source.Dir, absSource)
		}
		if fs.Target.Dir != m.Target {
			t.Errorf("filesystem[%d] target = %q, want %q", i, fs.Target.Dir, m.Target)
		}
	}
}

func TestCreateVM(t *testing.T) {
	requireIntegration(t)

	mounts := []Mount{{Source: mountDir(t), Target: "shared_data"}}
	name := createVM(t, testURI, mounts)
	verifyDomain(t, testURI, name, mounts)
}

func TestCreateSystemVM(t *testing.T) {
	requireIntegration(t)
	if os.Getenv("VMLITE_TEST_SYSTEM") != "1" {
		t.Skip("set VMLITE_TEST_SYSTEM=1 to run system-mode tests")
	}
	if err := ValidateConnection(systemURI); err != nil {
		t.Skipf("system libvirt unavailable: %v", err)
	}

	mounts := []Mount{{Source: mountDir(t), Target: "shared_data"}}
	name := createVM(t, systemURI, mounts)
	verifyDomain(t, systemURI, name, mounts)
}

func TestUpIdempotent(t *testing.T) {
	requireIntegration(t)

	name := uniqueName()
	registerCleanup(t, testURI, name)
	cfg := noInstallConfig(name, nil)
	path := writeConfig(t, cfg)

	if out, err := runVmlite(t, testURI, path); err != nil {
		t.Fatalf("first up failed: %v\n%s", err, out)
	}
	out, err := runVmlite(t, testURI, path)
	if err != nil {
		t.Fatalf("second up failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already running") {
		t.Fatalf("second up was not a no-op:\n%s", out)
	}
	if _, err := virsh(t, testURI, "dominfo", name); err != nil {
		t.Fatalf("domain is missing after idempotent up: %v", err)
	}
}

func TestNameCollisionWithoutState(t *testing.T) {
	requireIntegration(t)

	name := uniqueName()
	registerCleanup(t, testURI, name)
	firstPath := writeConfig(t, noInstallConfig(name, nil))
	if out, err := runVmlite(t, testURI, firstPath); err != nil {
		t.Fatalf("first up failed: %v\n%s", err, out)
	}

	secondPath := writeConfig(t, noInstallConfig(name, nil))
	out, err := runVmlite(t, testURI, secondPath)
	if err == nil {
		t.Fatalf("up succeeded without state for an existing domain:\n%s", out)
	}
	if !strings.Contains(out, "already exists") {
		t.Fatalf("unexpected collision error:\n%s", out)
	}
	if _, err := virsh(t, testURI, "dominfo", name); err != nil {
		t.Fatalf("original domain disappeared after collision: %v", err)
	}
}

func TestConfigDrift(t *testing.T) {
	requireIntegration(t)

	name := uniqueName()
	registerCleanup(t, testURI, name)
	cfg := noInstallConfig(name, nil)
	path := writeConfig(t, cfg)

	if out, err := runVmlite(t, testURI, path); err != nil {
		t.Fatalf("first up failed: %v\n%s", err, out)
	}

	cfg.Memory = 512
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal drifted config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write drifted config: %v", err)
	}

	out, err := runVmlite(t, testURI, path)
	if err == nil {
		t.Fatalf("up accepted config drift:\n%s", out)
	}
	if !strings.Contains(out, "config or connection changed") {
		t.Fatalf("unexpected drift error:\n%s", out)
	}

	out, err = runVmliteArgs(t, []string{"--connect", testURI, "--recreate", path}, nil)
	if err != nil {
		t.Fatalf("--recreate failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created successfully") {
		t.Fatalf("unexpected recreate output:\n%s", out)
	}
}

func TestLifecycleStatusDestroy(t *testing.T) {
	requireIntegration(t)

	name := uniqueName()
	registerCleanup(t, testURI, name)
	path := writeConfig(t, noInstallConfig(name, nil))

	if out, err := runVmlite(t, testURI, path); err != nil {
		t.Fatalf("up failed: %v\n%s", err, out)
	}
	stateFile := filepath.Join(filepath.Dir(path), ".vmlite", name+".json")
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file was not written: %v", err)
	}

	out, err := runVmliteArgs(t, []string{"status", path}, nil)
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "running") {
		t.Fatalf("status did not report running:\n%s", out)
	}

	out, err = runVmliteArgs(t, []string{"destroy", path}, nil)
	if err == nil {
		t.Fatalf("destroy without --force succeeded:\n%s", out)
	}
	if !strings.Contains(out, "use --force") {
		t.Fatalf("unexpected destroy safety error:\n%s", out)
	}

	out, err = runVmliteArgs(t, []string{"destroy", "--force", path}, nil)
	if err != nil {
		t.Fatalf("destroy --force failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("state file still exists after destroy: %v", err)
	}
	if _, err := virsh(t, testURI, "dominfo", name); err == nil {
		t.Fatalf("domain still exists after destroy")
	}

	out, err = runVmliteArgs(t, []string{"status", path}, nil)
	if err == nil {
		t.Fatalf("status succeeded after destroy:\n%s", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 3 {
		t.Fatalf("status exit = %v, want exit code 3", err)
	}
}

func TestImageChecksumMismatch(t *testing.T) {
	data := []byte("not-a-real-image")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	name := uniqueName()
	cfg := Config{
		Name:      name,
		Memory:    256,
		CPUs:      1,
		OSVariant: "debian12",
		Source: Source{
			Type:   sourceCloudImage,
			URL:    server.URL + "/image.qcow2",
			SHA256: strings.Repeat("0", 64),
			Cache:  t.TempDir(),
		},
		Disk:     Disk{Size: 1},
		Network:  Network{Mode: networkModeAuto},
		Graphics: Graphics{Type: graphicsNone},
	}
	path := writeConfig(t, cfg)

	out, err := runVmlite(t, testURI, path)
	if err == nil {
		t.Fatalf("up succeeded with a bad checksum:\n%s", out)
	}
	if !strings.Contains(out, "sha256 mismatch") {
		t.Fatalf("unexpected checksum error:\n%s", out)
	}
	stateFile := filepath.Join(filepath.Dir(path), ".vmlite", name+".json")
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("state was written after checksum failure: %v", err)
	}
}

func TestOfflineMissingImage(t *testing.T) {
	name := uniqueName()
	cfg := Config{
		Name:      name,
		Memory:    256,
		CPUs:      1,
		OSVariant: "debian12",
		Source: Source{
			Type:   sourceCloudImage,
			URL:    "https://example.invalid/image.qcow2",
			SHA256: strings.Repeat("a", 64),
			Cache:  t.TempDir(),
		},
		Disk:     Disk{Size: 1},
		Network:  Network{Mode: networkModeAuto},
		Graphics: Graphics{Type: graphicsNone},
	}
	path := writeConfig(t, cfg)

	out, err := runVmliteArgs(t, []string{"--connect", testURI, "--offline", path}, nil)
	if err == nil {
		t.Fatalf("up --offline succeeded for a missing image:\n%s", out)
	}
	if !strings.Contains(out, "offline") {
		t.Fatalf("unexpected offline error:\n%s", out)
	}
}

func TestUpURLImage(t *testing.T) {
	requireIntegration(t)
	if err := ValidateConnection(sessionURI); err != nil {
		t.Skipf("session libvirt unavailable: %v", err)
	}

	dir := t.TempDir()
	base := filepath.Join(dir, "base.qcow2")
	create := exec.Command("qemu-img", "create", "-f", "qcow2", base, "1G")
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create base image: %v: %s", err, out)
	}
	data, err := os.ReadFile(base)
	if err != nil {
		t.Fatalf("read base image: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	name := uniqueName()
	registerCleanup(t, sessionURI, name)
	cache := filepath.Join(dir, "cache")
	cfg := Config{
		Name:      name,
		Memory:    256,
		CPUs:      1,
		OSVariant: "debian12",
		Source: Source{
			Type:   sourceCloudImage,
			URL:    server.URL + "/base.qcow2",
			SHA256: shaHex(data),
			Cache:  cache,
		},
		Disk:     Disk{Size: 2},
		Network:  Network{Mode: networkModeUser},
		Graphics: Graphics{Type: graphicsNone},
	}
	path := writeConfig(t, cfg)

	out, err := runVmlite(t, sessionURI, path)
	if err != nil {
		t.Fatalf("up URL image failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created successfully") {
		t.Fatalf("unexpected up output:\n%s", out)
	}

	stateFile := filepath.Join(filepath.Dir(path), ".vmlite", name+".json")
	st, err := loadState(stateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st == nil || st.ImageSHA256 != shaHex(data) {
		t.Fatalf("state image checksum = %+v", st)
	}
	if st.ImagePath == "" {
		t.Fatal("state has no resolved image path")
	}
	if _, err := virsh(t, sessionURI, "dominfo", name); err != nil {
		t.Fatalf("domain missing after URL up: %v", err)
	}

	out, err = runVmliteArgs(t, []string{"destroy", "--force", path}, nil)
	if err != nil {
		t.Fatalf("destroy URL VM: %v\n%s", err, out)
	}
	if _, err := os.Stat(st.ImagePath); err != nil {
		t.Fatalf("image cache was removed by destroy: %v", err)
	}
}

func TestMissingMountSource(t *testing.T) {
	requireIntegration(t)

	name := uniqueName()
	cfg := noInstallConfig(name, []Mount{{Source: filepath.Join(t.TempDir(), "missing"), Target: "data"}})
	path := writeConfig(t, cfg)

	out, err := runVmlite(t, testURI, path)
	if err == nil {
		t.Fatalf("vmlite succeeded, want error:\n%s", out)
	}
	if !strings.Contains(out, "not accessible") {
		t.Fatalf("output does not mention inaccessible mount:\n%s", out)
	}
	if _, err := virsh(t, testURI, "dominfo", name); err == nil {
		t.Fatalf("domain %s was defined despite the bad mount", name)
	}
}

func TestConnectionOverrides(t *testing.T) {
	requireIntegration(t)
	if err := ValidateConnection(sessionURI); err != nil {
		t.Skipf("session libvirt unavailable: %v", err)
	}

	// A missing mount source makes the run fail after the connection has been
	// selected, so we can assert on the selected URI without creating a VM.
	cfg := noInstallConfig(uniqueName(), []Mount{{Source: filepath.Join(t.TempDir(), "missing"), Target: "data"}})
	path := writeConfig(t, cfg)

	t.Run("environment", func(t *testing.T) {
		out, err := runVmliteArgs(t, []string{path}, map[string]string{"VMLITE_CONNECT": sessionURI})
		if err == nil {
			t.Fatalf("vmlite succeeded, want mount error:\n%s", out)
		}
		if !strings.Contains(out, "Using libvirt connection: "+sessionURI) {
			t.Fatalf("did not use VMLITE_CONNECT:\n%s", out)
		}
	})

	t.Run("flag beats environment", func(t *testing.T) {
		out, err := runVmliteArgs(t, []string{"--connect", sessionURI, path}, map[string]string{"VMLITE_CONNECT": systemURI})
		if err == nil {
			t.Fatalf("vmlite succeeded, want mount error:\n%s", out)
		}
		if !strings.Contains(out, "Using libvirt connection: "+sessionURI) {
			t.Fatalf("--connect did not override the environment:\n%s", out)
		}
	})
}

func fakeVirtInstall(t *testing.T) (dir, logPath string) {
	t.Helper()
	dir = t.TempDir()
	logPath = filepath.Join(dir, "args.log")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > \"%s\"\nexit 0\n", logPath)
	if err := os.WriteFile(filepath.Join(dir, "virt-install"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake virt-install: %v", err)
	}
	return dir, logPath
}

func readArgs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestNoInstallArgsCapture(t *testing.T) {
	requireIntegration(t)

	binDir, logPath := fakeVirtInstall(t)
	cfg := noInstallConfig(uniqueName(), nil)
	path := writeConfig(t, cfg)

	env := map[string]string{
		"PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	out, err := runVmliteArgs(t, []string{"--connect", testURI, path}, env)
	if err != nil {
		t.Fatalf("vmlite failed: %v\n%s", err, out)
	}

	args := readArgs(t, logPath)
	for _, want := range []string{"--install", "no_install=yes", "--noautoconsole", "--network"} {
		if !containsArg(args, want) {
			t.Errorf("captured args missing %q: %q", want, args)
		}
	}
	if containsArg(args, "--location") {
		t.Errorf("captured args unexpectedly contain --location: %q", args)
	}
}

func TestInstallURLArgsCapture(t *testing.T) {
	requireIntegration(t)

	binDir, logPath := fakeVirtInstall(t)
	cfg := Config{
		Name:      uniqueName(),
		Memory:    256,
		CPUs:      1,
		OSVariant: "debian12",
		Source: Source{
			Type:      sourceInstallTree,
			URL:       "https://example.invalid/debian/",
			ExtraArgs: "console=ttyS0,115200n8",
		},
		Disk:        Disk{Size: 1},
		Network:     Network{Mode: networkModeAuto},
		Graphics:    Graphics{Type: graphicsNone},
		Autoconsole: true,
	}
	path := writeConfig(t, cfg)

	env := map[string]string{
		"PATH": binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	out, err := runVmliteArgs(t, []string{"--connect", testURI, path}, env)
	if err != nil {
		t.Fatalf("vmlite failed: %v\n%s", err, out)
	}

	args := readArgs(t, logPath)
	for _, want := range []string{"--location", "https://example.invalid/debian/", "--extra-args", "console=ttyS0,115200n8"} {
		if !containsArg(args, want) {
			t.Errorf("captured args missing %q: %q", want, args)
		}
	}
	for _, unwanted := range []string{"--install", "--noautoconsole"} {
		if containsArg(args, unwanted) {
			t.Errorf("captured args unexpectedly contain %q: %q", unwanted, args)
		}
	}
}

# Coding Agent Instructions: Building a Lightweight Go KVM/Libvirt CLI (`vmlite`)

You are tasked with implementing `vmlite`, a minimal, zero-external-dependency Go CLI tool designed to replace Vagrant by managing local KVM/QEMU virtual machines directly via `virt-install` and `virsh`. 

The tool must read a JSON configuration file (`vm.json`), run pre-flight host checks, dynamically resolve libvirt connection URIs (`qemu:///system` with a fallback to `qemu:///session`), and execute provisioning commands cleanly.

---

## 1. Project Specifications & Constraints
- **Language:** Go (Latest stable version).
- **Dependencies:** **Zero third-party dependencies** (Strictly use Go standard library packages: `encoding/json`, `os`, `os/exec`, `fmt`, `strconv`, `errors`).
- **Target OS:** Linux (Host system supporting KVM/libvirt).
- **Binary Name:** `vmlite`

---

## 2. Expected Project Structure
```text
vmlite/
├── main.go
├── config.go
├── preflight.go
├── executor.go
└── vm.json (sample configuration)
```

---

## 3. Implementation Requirements by Module

### A. Configuration (`config.go`)
Define the data structures to parse a JSON configuration file.

- **Structs:**
  ```go
  type Config struct {
      Name      string  `json:"name"`
      Memory    int     `json:"memory"`    // in MiB
      CPUs      int     `json:"cpus"`
      DiskSize  int     `json:"disk_size"` // in GiB
      OSVariant string  `json:"os_variant"`
      Mounts    []Mount `json:"mounts"`
  }

  type Mount struct {
      Source string `json:"source"`
      Target string `json:"target"`
  }
  ```
- **Function:** `LoadConfig(path string) (*Config, error)`
  - Opens the file path using `os.Open`.
  - Decodes JSON using `json.NewDecoder`.
  - Returns the populated `Config` pointer or a descriptive error.

---

### B. Pre-flight Validation & URI Resolution (`preflight.go`)
Verify host readiness and determine whether to use system or session mode.

- **Function:** `RunPreflightChecks() (string, error)`
  1. **Binary Check:** Verify `virt-install` and `virsh` exist in the system `PATH` using `exec.LookPath`.
  2. **Hardware Check:** Verify `/dev/kvm` exists using `os.Stat`. If missing, return an error explaining that hardware virtualization may be disabled or kernel modules unloaded.
  3. **Daemon & URI Resolution:**
     - Test connection to the system daemon: `virsh -c qemu:///system nodeinfo`.
     - If successful, return `"qemu:///system"`.
     - If it fails (permission/polkit error), test session daemon: `virsh -c qemu:///session nodeinfo`.
     - If session succeeds, print a warning to `os.Stderr` and return `"qemu:///session"`.
     - If both fail, return an error stating no libvirt daemons are accessible.

---

### C. Subprocess Execution & Provisioning (`executor.go`)
Translate the configuration and connection URI into an active `virt-install` command.

- **Function:** `RunVirtInstall(cfg *Config, libvirtURI string) error`
  - Build the argument slice dynamically:
    ```go
    args := []string{
        "--connect", libvirtURI,
        "--name", cfg.Name,
        "--memory", strconv.Itoa(cfg.Memory),
        "--vcpus", strconv.Itoa(cfg.CPUs),
        "--disk", fmt.Sprintf("size=%d", cfg.DiskSize),
        "--os-variant", cfg.OSVariant,
        "--graphics", "none",
        "--extra-args", "console=ttyS0,115200n8",
    }
    ```
  - **Network Handling:**
    - If `libvirtURI == "qemu:///system"`, append `--network`, `network=default`.
    - If `libvirtURI == "qemu:///session"`, append `--network`, `user` (to bypass system bridge restrictions).
  - **Filesystem Mounts:**
    - Iterate over `cfg.Mounts`: append `--filesystem`, `type=mount,source=...,target=...,accessmode=mapped`.
  - **Execution:**
    - Initialize `exec.Command("virt-install", args...)`.
    - Wire `cmd.Stdout = os.Stdout`, `cmd.Stderr = os.Stderr`, and `cmd.Stdin = os.Stdin`.
    - Run the command and return any resulting error.

---

### D. Main Entrypoint (`main.go`)
Tie all modules together with clean CLI argument parsing and error exit handling.

- **Flow:**
  1. Expects path to config file as an argument (default to `vm.json` if none provided).
  2. Call `LoadConfig`. Exit with non-zero status on error.
  3. Call `RunPreflightChecks()`. Exit with non-zero status on error.
  4. Call `RunVirtInstall`. Exit with non-zero status on error.
  5. Print success message upon completion.

---

## 4. Sample `vm.json` for Testing
Create a default sample file for agent verification:
```json
{
  "name": "test-vm",
  "memory": 2048,
  "cpus": 2,
  "disk_size": 20,
  "os_variant": "debian12",
  "mounts": [
    {
      "source": "./data",
      "target": "shared_data"
    }
  ]
}
```

---

## 5. Acceptance Criteria
- Code compiles cleanly with `go build -o vmlite .`.
- Zero external imports in `go.mod`.
- Gracefully handles unprivileged execution by falling back to `qemu:///session`.
- Streams terminal output live during guest installation.
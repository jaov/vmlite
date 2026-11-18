package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
)

type command struct {
	Name     string
	Summary  string
	Usage    string
	Examples []string
	Flags    func() *flag.FlagSet
}

func commands() []command {
	return []command{
		{
			Name:    "up",
			Summary: "Create or start the VM",
			Usage:   "vmlite up [options] [config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _, _, _ := upFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite up",
				"vmlite up vm.json",
				"vmlite up --connect qemu:///system vm.json",
				"vmlite up --recreate vm.json",
			},
		},
		{
			Name:    "init",
			Summary: "Write a starter vm.json from an example template",
			Usage:   "vmlite init [options] [config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _, _, _ := initFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite init",
				"vmlite init --template minimal",
				"vmlite init --template arch-script --name arch",
				"vmlite init --list",
			},
		},
		{
			Name:    "provision",
			Summary: "Run pending provisioners or finish an installer",
			Usage:   "vmlite provision [options] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _, _ := provisionFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite provision",
				"vmlite provision vm.json",
				"vmlite provision dev",
				"vmlite provision --force dev",
				"vmlite provision --timeout 45m dev",
			},
		},
		{
			Name:    "status",
			Summary: "Show VM, installer, IP, and provisioner state",
			Usage:   "vmlite status [options] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _ := statusFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite status",
				"vmlite status vm.json",
				"vmlite status dev",
				"vmlite status --json dev",
			},
		},
		{
			Name:    "inspect",
			Summary: "Show detailed VM, disk, network, and libvirt state",
			Usage:   "vmlite inspect [--json|--xml] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _ := inspectFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite inspect",
				"vmlite inspect vm.json",
				"vmlite inspect dev",
				"vmlite inspect --json dev",
				"vmlite inspect --xml dev",
			},
		},
		{
			Name:    "start",
			Summary: "Start a stopped VM",
			Usage:   "vmlite start [options] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _ := startFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite start vm.json",
				"vmlite start dev",
			},
		},
		{
			Name:    "stop",
			Summary: "Gracefully stop a running VM",
			Usage:   "vmlite stop [options] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _, _ := stopFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite stop vm.json",
				"vmlite stop dev",
				"vmlite stop --timeout 5m dev",
				"vmlite stop --force dev",
			},
		},
		{
			Name:    "restart",
			Summary: "Reboot a running VM or start a stopped VM",
			Usage:   "vmlite restart [options] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _ := restartFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite restart vm.json",
				"vmlite restart dev",
			},
		},
		{
			Name:    "ssh",
			Summary: "Open a shell or run a command in the guest",
			Usage:   "vmlite ssh [options] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _, _, _ := sshFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite ssh vm.json",
				"vmlite ssh dev",
				"vmlite ssh --command 'uname -a' dev",
			},
		},
		{
			Name:    "scp",
			Summary: "Copy a host file into the guest",
			Usage:   "vmlite scp [options] <local> <remote> [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _, _, _ := scpFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite scp ./app.conf /etc/app.conf vm.json",
				"vmlite scp ./app.conf /etc/app.conf dev",
				"vmlite scp --recursive ./assets /opt/assets dev",
			},
		},
		{
			Name:    "console",
			Summary: "Attach to serial or print graphical details",
			Usage:   "vmlite console [options] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _, _, _ := consoleFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite console vm.json",
				"vmlite console dev",
				"vmlite console --type vnc dev",
				"vmlite console --log dev",
			},
		},
		{
			Name:    "list",
			Summary: "List managed VMs in the state directory",
			Usage:   "vmlite list [options]",
			Flags: func() *flag.FlagSet {
				fs, _, _ := listFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite list",
				"vmlite list --json",
			},
		},
		{
			Name:    "destroy",
			Summary: "Remove the VM and managed files",
			Usage:   "vmlite destroy [options] [name|config.json]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _ := destroyFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite destroy vm.json",
				"vmlite destroy dev",
				"vmlite destroy --force dev",
			},
		},
		{
			Name:    "validate",
			Summary: "Validate a config without changing anything",
			Usage:   "vmlite validate [config.json]",
			Flags: func() *flag.FlagSet {
				return newStateFlagSet("validate")
			},
			Examples: []string{
				"vmlite validate",
				"vmlite validate vm.json",
			},
		},
		{
			Name:    "schema",
			Summary: "Print the JSON Schema for vmlite configs",
			Usage:   "vmlite schema",
			Examples: []string{
				"vmlite schema > vmlite.schema.json",
			},
		},
		{
			Name:    "install",
			Summary: "Install host dependencies and vmlite",
			Usage:   "vmlite install [options]",
			Flags: func() *flag.FlagSet {
				fs, _, _, _, _, _, _ := installFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite install",
				"vmlite install --dry-run",
				"vmlite install --no-deps",
			},
		},
		{
			Name:    "doctor",
			Summary: "Check host readiness",
			Usage:   "vmlite doctor [options]",
			Flags: func() *flag.FlagSet {
				fs, _ := doctorFlagSet()
				return fs
			},
			Examples: []string{
				"vmlite doctor",
			},
		},
		{
			Name:    "version",
			Summary: "Print the vmlite version",
			Usage:   "vmlite version",
			Examples: []string{
				"vmlite version",
				"vmlite --version",
			},
		},
		{
			Name:    "help",
			Summary: "Show help for a command",
			Usage:   "vmlite help [command]",
			Examples: []string{
				"vmlite help",
				"vmlite help up",
			},
		},
	}
}

func lookupCommand(name string) (command, bool) {
	for _, cmd := range commands() {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return command{}, false
}

func runHelp(args []string) error {
	if len(args) > 1 {
		return errUsage
	}
	if len(args) == 0 {
		writeTopHelp(os.Stdout)
		return nil
	}
	cmd, ok := lookupCommand(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "vmlite: unknown command %q\n\n", args[0])
		writeTopHelp(os.Stderr)
		return errUsage
	}
	writeCommandHelp(os.Stdout, cmd)
	return nil
}

func writeTopHelp(w io.Writer) {
	fmt.Fprintln(w, "vmlite - reproducible local KVM guests")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  vmlite [command] [options] [config.json|name]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, cmd := range commands() {
		fmt.Fprintf(w, "  %-10s %s\n", cmd.Name, cmd.Summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags accept either -name or --name.")
	fmt.Fprintln(w, "Run 'vmlite help <command>' for command flags and examples.")
	fmt.Fprintln(w, "VM commands accept a managed VM name instead of a config path.")
}

func writeCommandHelp(w io.Writer, cmd command) {
	fmt.Fprintf(w, "vmlite %s - %s\n\n", cmd.Name, cmd.Summary)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintf(w, "  %s\n", cmd.Usage)
	if cmd.Flags != nil {
		fs := cmd.Flags()
		fs.SetOutput(w)
		var lines int
		fs.VisitAll(func(*flag.Flag) { lines++ })
		if lines > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Options:")
			fs.PrintDefaults()
		}
	}
	if len(cmd.Examples) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Examples:")
		for _, example := range cmd.Examples {
			fmt.Fprintf(w, "  %s\n", example)
		}
	}
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

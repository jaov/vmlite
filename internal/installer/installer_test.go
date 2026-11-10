package installer

import (
	"strings"
	"testing"
)

func TestUbuntuDefaultSpec(t *testing.T) {
	impl, err := Lookup(UbuntuAutoinstall)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	spec, err := impl.Spec(Input{
		Hostname: "demo-vm",
		SSHUser:  "vmlite",
		SSHKey:   "ssh-ed25519 AAAA test",
	})
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if spec.Type != UbuntuAutoinstall || spec.Seed == nil {
		t.Fatalf("spec = %+v", spec)
	}
	if spec.Seed.Label != "cidata" || spec.SeedDevice != "disk" {
		t.Fatalf("seed = %+v", spec.Seed)
	}
	if got := strings.Join(spec.BootOrder, ","); got != "hd,cdrom" {
		t.Fatalf("boot order = %q", got)
	}
	if got := strings.Join(spec.KernelArgs, " "); got != "autoinstall" {
		t.Fatalf("kernel args = %q", got)
	}

	var userData, metaData string
	for _, f := range spec.Seed.Files {
		switch f.Name {
		case "user-data":
			userData = string(f.Data)
		case "meta-data":
			metaData = string(f.Data)
		}
	}
	for _, want := range []string{
		"autoinstall:", "version: 1", "hostname: demo-vm", "username: vmlite",
		"authorized-keys:", "ssh-ed25519 AAAA test", "ALL=(ALL) NOPASSWD:ALL",
		"storage:", "name: direct",
	} {
		if !strings.Contains(userData, want) {
			t.Errorf("user-data missing %q:\n%s", want, userData)
		}
	}
	if !strings.Contains(metaData, "instance-id: demo-vm") {
		t.Errorf("meta-data = %q", metaData)
	}
}

func TestUbuntuAnswerFilePlaceholders(t *testing.T) {
	impl, _ := Lookup(UbuntuAutoinstall)
	spec, err := impl.Spec(Input{
		Hostname:   "demo",
		SSHUser:    "deploy",
		SSHKey:     "ssh-ed25519 KEY",
		AnswerFile: []byte("host={{VMLITE_HOSTNAME}} user={{VMLITE_SSH_USER}} key={{VMLITE_SSH_AUTHORIZED_KEY}}\n"),
	})
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if got := string(spec.Seed.Files[0].Data); got != "host=demo user=deploy key=ssh-ed25519 KEY\n" {
		t.Fatalf("user-data = %q", got)
	}
}

func TestUbuntuNoKeyDoesNotInstallSSH(t *testing.T) {
	impl, _ := Lookup(UbuntuAutoinstall)
	spec, err := impl.Spec(Input{Hostname: "demo", SSHUser: "vmlite"})
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	userData := string(spec.Seed.Files[0].Data)
	if !strings.Contains(userData, "install-server: false") {
		t.Fatalf("user-data should disable sshd without a key:\n%s", userData)
	}
	if strings.Contains(userData, "authorized-keys") {
		t.Fatalf("user-data should not contain authorized-keys:\n%s", userData)
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, err := Lookup("nope"); err == nil || !strings.Contains(err.Error(), "unsupported installer.type") {
		t.Fatalf("error = %v", err)
	}
}

func TestUbuntuRejectsUnsafeUser(t *testing.T) {
	impl, _ := Lookup(UbuntuAutoinstall)
	_, err := impl.Spec(Input{Hostname: "demo", SSHUser: `bad"user`, SSHKey: "ssh-ed25519 AAAA"})
	if err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("error = %v", err)
	}
}

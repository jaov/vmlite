package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectPools(t *testing.T) {
	binDir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *' pool-list '*) printf 'default\nvmlite-dev\n' ;;
  *' pool-dumpxml default'*) cat <<'EOF'
<pool type='dir'><target><path>/var/lib/libvirt/images</path></target></pool>
EOF
    ;;
  *' pool-dumpxml vmlite-dev'*) cat <<'EOF'
<pool type='dir'><target><path>/project/.vmlite/dev</path></target></pool>
EOF
    ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "virsh"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake virsh: %v", err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	pools, err := inspectPools("qemu:///system")
	if err != nil {
		t.Fatalf("inspectPools: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("pools = %+v", pools)
	}
	if pools[0].Name != "default" || pools[0].Vmlite {
		t.Fatalf("default pool = %+v", pools[0])
	}
	if pools[1].Name != "vmlite-dev" || !pools[1].Vmlite || pools[1].Path != "/project/.vmlite/dev" {
		t.Fatalf("vmlite pool = %+v", pools[1])
	}
}

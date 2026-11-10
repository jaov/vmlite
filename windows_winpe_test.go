package main

import (
	"os"
	"strings"
	"testing"
)

func TestWinPEStartnet(t *testing.T) {
	data, err := os.ReadFile("scripts/winpe/startnet.cmd")
	if err != nil {
		t.Fatalf("read startnet.cmd: %v", err)
	}
	script := string(data)
	for _, want := range []string{
		"wpeinit",
		"select disk 1",
		"format fs=fat32 label=VMLITE quick",
		"vmlite-winpe-ok.txt",
		"wpeutil shutdown",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("startnet.cmd missing %q:\n%s", want, script)
		}
	}
}

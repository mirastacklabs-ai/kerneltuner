package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/apply"
)

// Q01: the plan table never prints control characters.
func TestPrintPlanStripsControlChars(t *testing.T) {
	plan := &apply.Plan{
		Ops: []apply.Op{{RuleID: "r\x1b[2J", Kind: "sysfs", Param: "/sys/block/{item}/queue/scheduler",
			Target: "sda\x1b]0;pwn\x07\nFAKE ROW", Proposed: "none\r", LivePath: "/sys/x\x9b"}},
		Skipped:  []apply.Skip{{RuleID: "s\x1b", Reason: "bad\x1b[31m"}},
		Warnings: []string{"w\x1b"},
		Blocked:  []string{"b\x1b"},
	}
	var out bytes.Buffer
	printPlan(&out, plan)
	for _, r := range out.String() {
		if r != '\n' && r != '\t' && (r < 0x20 || (r >= 0x7f && r <= 0x9f)) {
			t.Fatalf("control character %q in output:\n%q", r, out.String())
		}
	}
	if strings.Count(out.String(), "\n") != 5 { // header, op, skipped, warning, conflict: no injected rows
		t.Fatalf("injected line breaks: %q", out.String())
	}
}

// Q19: generate --out never writes through symlinks.
func TestWriteNoFollow(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, "90-kerneltuner.conf")); err != nil {
		t.Fatal(err)
	}
	if err := writeNoFollow(dir, "90-kerneltuner.conf", []byte("x"), 0o644); err == nil {
		t.Fatal("wrote through a file symlink")
	}
	if err := os.Symlink(outside, filepath.Join(dir, "limits.d")); err != nil {
		t.Fatal(err)
	}
	if err := writeNoFollow(dir, "limits.d/90.conf", []byte("x"), 0o644); err == nil {
		t.Fatal("wrote through a directory symlink")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("victim changed: %q", b)
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 1 {
		t.Fatalf("files escaped: %v", ents)
	}
	// the normal case still works, including nested directories
	if err := writeNoFollow(dir, "systemd/a.service.d/90.conf", []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "systemd/a.service.d/90.conf")); string(b) != "ok" {
		t.Fatalf("content %q", b)
	}
}

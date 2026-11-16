package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
)

func fakeRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "proc/sys/net/core/somaxconn")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("128\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runApplyCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestApplyRefusals(t *testing.T) {
	old := geteuid
	geteuid = func() int { return 1000 }
	t.Cleanup(func() { geteuid = old })

	if _, err := runApplyCLI(t, "apply", "--yes"); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("non-root without --root: %v", err)
	}
	root := fakeRoot(t)
	if _, err := runApplyCLI(t, "--root", root, "apply"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("without --yes: %v", err)
	}
	if _, err := runApplyCLI(t, "--root", root, "rollback", "--yes"); err == nil || !strings.Contains(err.Error(), "no snapshots") {
		t.Fatalf("rollback latest with none: %v", err)
	}
}

func TestApplyRollbackEndToEnd(t *testing.T) {
	root := fakeRoot(t)
	g := &globalOpts{RootDir: root}
	recs := []advisor.Recommendation{
		{RuleID: "net-somaxconn", Kind: "sysctl", Param: "net.core.somaxconn", Proposed: "4096", Status: advisor.StatusRecommend},
		{RuleID: "other", Kind: "sysctl", Param: "net.core.somaxconn", Proposed: "1", Status: advisor.StatusRecommend},
	}
	var out, errOut bytes.Buffer
	if err := runApply(&out, &errOut, g, recs, []string{"net-somaxconn", "missing"}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dry run") || !strings.Contains(errOut.String(), "missing") {
		t.Fatalf("out=%q err=%q", out.String(), errOut.String())
	}
	live := filepath.Join(root, "proc/sys/net/core/somaxconn")
	if b, _ := os.ReadFile(live); string(b) != "128\n" {
		t.Fatalf("dry-run wrote: %q", b)
	}

	out.Reset()
	if err := runApply(&out, &errOut, g, recs, []string{"net-somaxconn"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "undo with: kerneltuner rollback ") {
		t.Fatalf("out=%q", out.String())
	}
	if b, _ := os.ReadFile(live); string(b) != "4096\n" {
		t.Fatalf("not applied: %q", b)
	}

	if o, err := runApplyCLI(t, "--root", root, "rollback", "--list"); err != nil || !strings.Contains(o, "1 live value") {
		t.Fatalf("list: %q %v", o, err)
	}
	if o, err := runApplyCLI(t, "--root", root, "rollback", "--yes"); err != nil || !strings.Contains(o, "rolled back") {
		t.Fatalf("rollback: %q %v", o, err)
	}
	if b, _ := os.ReadFile(live); string(b) != "128\n" {
		t.Fatalf("not restored: %q", b)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/sysctl.d/90-kerneltuner.conf")); !os.IsNotExist(err) {
		t.Fatalf("sysctl.d file should be gone: %v", err)
	}
}

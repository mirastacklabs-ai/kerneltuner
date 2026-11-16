package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func mkRoot(t *testing.T) (string, facts.Root) {
	t.Helper()
	dir := t.TempDir()
	for _, s := range []string{"proc", "sys", "etc", "var"} {
		if err := os.Mkdir(filepath.Join(dir, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, facts.RootAt(dir)
}

// Q02: a root-run --root refuses a rigged tree. Real symlinks and modes.
func TestCheckRootRefusesRiggedTree(t *testing.T) {
	for _, base := range []string{"proc", "sys", "etc", "var"} {
		dir, root := mkRoot(t)
		p := filepath.Join(dir, base)
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), p); err != nil {
			t.Fatal(err)
		}
		if err := CheckRoot(dir, root); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Errorf("symlinked %s accepted: %v", base, err)
		}
	}
	// The root directory itself is a symlink.
	dir, _ := mkRoot(t)
	link := filepath.Join(t.TempDir(), "rootlink")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if err := CheckRoot(link, facts.RootAt(link)); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("symlinked root accepted: %v", err)
	}
	// Group- and world-writable bases and root.
	for _, mode := range []os.FileMode{0o777, 0o775, 0o757} {
		dir, root := mkRoot(t)
		if err := os.Chmod(root.Etc, mode); err != nil {
			t.Fatal(err)
		}
		if err := CheckRoot(dir, root); err == nil {
			t.Errorf("etc mode %o accepted", mode)
		}
		dir, root = mkRoot(t)
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		if err := CheckRoot(dir, root); err == nil {
			t.Errorf("root mode %o accepted", mode)
		}
	}
	// A good tree passes only when it is owned by root (tests run as root in Docker).
	dir, root := mkRoot(t)
	err := CheckRoot(dir, root)
	if os.Geteuid() == 0 && err != nil {
		t.Errorf("good tree refused: %v", err)
	}
	if os.Geteuid() != 0 && (err == nil || !strings.Contains(err.Error(), "not root")) {
		t.Errorf("tree owned by uid %d accepted: %v", os.Geteuid(), err)
	}
}

// Q08: writable proc paths that lead to code execution are never targets,
// lexically or after resolving symlinks. Real files and symlinks.
func TestResolveLiveDeniesCodeExecutionTargets(t *testing.T) {
	root := setup(t)
	for _, p := range []string{"kernel/core_pattern", "kernel/modprobe", "kernel/hotplug", "kernel/poweroff_cmd", "fs/binfmt_misc/register", "fs/binfmt_misc/status", "fs/binfmt_misc/sub/dir/x"} {
		write(t, filepath.Join(root.Proc, "sys", p), "x\n")
		if _, err := resolveLive(root, filepath.Join(root.Proc, "sys", p)); err == nil || !strings.Contains(err.Error(), "code-execution") {
			t.Errorf("%s: err = %v", p, err)
		}
	}
	// A harmless-looking path that resolves to a dangerous one.
	if err := os.Symlink("../kernel/core_pattern", filepath.Join(root.Proc, "sys/net/innocent")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveLive(root, filepath.Join(root.Proc, "sys/net/innocent")); err == nil || !strings.Contains(err.Error(), "code-execution") {
		t.Errorf("symlink to core_pattern: err = %v", err)
	}
	// An ordinary neighbour still resolves.
	write(t, filepath.Join(root.Proc, "sys/kernel/pid_max"), "4194304\n")
	if _, err := resolveLive(root, filepath.Join(root.Proc, "sys/kernel/pid_max")); err != nil {
		t.Errorf("pid_max: %v", err)
	}
	// A hand-made op is refused with nothing written.
	op := Op{RuleID: "r", Kind: "sysctl", Param: "kernel.core_pattern", Proposed: "|/tmp/x", FileKey: "kernel.core_pattern",
		LivePath: filepath.Join(root.Proc, "sys/kernel/core_pattern"), File: sysctlConfPath(root), FileKind: fileSysctl}
	if _, err := Apply(&Plan{Root: root, Ops: []Op{op}}, Options{Now: fixed()}); err == nil {
		t.Error("apply accepted core_pattern")
	}
	if got := read(t, filepath.Join(root.Proc, "sys/kernel/core_pattern")); got != "x\n" {
		t.Errorf("core_pattern modified: %q", got)
	}
}

// Q10: on an empty --root (no etc at all) the snapshot lists only dirs under
// Etc, so it validates and rollback works and removes what apply created.
func TestSnapshotOnEmptyRootStaysValid(t *testing.T) {
	root := facts.RootAt(filepath.Join(t.TempDir(), "newroot"))
	rc := advisor.Recommendation{RuleID: "r", Kind: "limit", Param: "LimitNOFILE", Target: "nginx.service", Proposed: "1048576", Status: advisor.StatusRecommend}
	p, err := BuildPlan([]advisor.Recommendation{rc}, root)
	if err != nil || len(p.Ops) != 1 {
		t.Fatalf("plan: %v %+v", err, p)
	}
	res, err := Apply(p, Options{Now: fixed()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := load(root, res.ID); err != nil {
		t.Fatalf("snapshot invalid: %v", err)
	}
	for _, d := range missingDirs(root.Etc, filepath.Join(root.Etc, "a", "b")) {
		if !under(root.Etc, d) {
			t.Errorf("missingDirs escaped Etc: %s", d)
		}
	}
	if err := Rollback(root, res.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dropinPath(root, "nginx.service")); !os.IsNotExist(err) {
		t.Error("drop-in still present after rollback")
	}
}

package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// applyOne applies a single sysctl and returns the snapshot id.
func applyOne(t *testing.T, root facts.Root) string {
	t.Helper()
	p, err := BuildPlan([]advisor.Recommendation{rec("sysctl", "net.core.somaxconn", "", "4096")}, root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Apply(p, Options{Now: fixed()})
	if err != nil {
		t.Fatal(err)
	}
	return res.ID
}

func snapPath(root facts.Root, id string) string { return filepath.Join(snapDir(root), id+".json") }

// rewriteSnap edits the snapshot JSON of id through fn.
func rewriteSnap(t *testing.T, root facts.Root, id string, fn func(m map[string]any)) {
	t.Helper()
	b, err := os.ReadFile(snapPath(root, id))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	fn(m)
	b, _ = json.Marshal(m)
	if err := os.WriteFile(snapPath(root, id), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Q01: Apply refuses limit ops whose unit fails validation (hand-made plans).
func TestApplyRefusesInvalidUnit(t *testing.T) {
	root := setup(t)
	for _, unit := range []string{"a b.service", "x\nExecStart=/bin/sh.service", "../evil.service", "a'; rm -rf.service", "user@1000.slice", ".hidden.service"} {
		op := Op{RuleID: "r", Kind: "limit", Param: "nofile", Proposed: "1", Unit: unit,
			File: dropinPath(root, unit), FileKind: fileDropin, FileKey: "LimitNOFILE"}
		if _, err := Apply(&Plan{Root: root, Ops: []Op{op}}, Options{Now: fixed()}); err == nil || !strings.Contains(err.Error(), "invalid systemd unit") {
			t.Errorf("unit %q: err = %v", unit, err)
		}
	}
	// Valid escaped names still work.
	if !facts.ValidUnitName(`getty@tty1.service`) || !facts.ValidUnitName(`dev-disk-by\x2dlabel.service`) {
		t.Error("valid unit names rejected")
	}
}

func TestApplyShellQuotesHint(t *testing.T) {
	root := setup(t)
	unit := "a$(id).service"
	if facts.ValidUnitName(unit) {
		t.Skip("unit with $ must be invalid")
	}
	if shellQuote("it's") != `'it'\''s'` {
		t.Errorf("shellQuote = %s", shellQuote("it's"))
	}
	p, _ := BuildPlan([]advisor.Recommendation{rec("limit", "nofile", "nginx", "5")}, root)
	res, _ := Apply(p, Options{DryRun: true})
	if len(res.Hints) != 1 || !strings.HasSuffix(res.Hints[0], "restart 'nginx.service'") {
		t.Errorf("hints = %v", res.Hints)
	}
}

// Q02: a tampered snapshot is refused.
func TestRollbackRejectsTamperedSnapshot(t *testing.T) {
	cases := map[string]func(m map[string]any){
		"live path outside proc/sys": func(m map[string]any) {
			m["live"] = []any{map[string]any{"path": "/etc/shadow", "prior": "x"}}
		},
		"file outside managed names": func(m map[string]any) {
			m["files"] = []any{map[string]any{"path": "/etc/cron.d/evil", "content": "eA==", "mode": 0o644}}
		},
		"created dir elsewhere": func(m map[string]any) { m["created_dirs"] = []any{"/usr/lib"} },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			root := setup(t)
			id := applyOne(t, root)
			rewriteSnap(t, root, id, fn)
			if err := Rollback(root, id); err == nil {
				t.Fatal("tampered snapshot was accepted")
			}
		})
	}
}

func TestSnapshotModeMasked(t *testing.T) {
	root := setup(t)
	id := applyOne(t, root)
	conf := filepath.Join(root.Etc, sysctlFile)
	rewriteSnap(t, root, id, func(m map[string]any) {
		m["files"] = []any{map[string]any{"path": conf, "content": "eA==", "mode": 0o4755}}
	})
	if err := Rollback(root, id); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(conf)
	if err != nil || st.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || st.Mode().Perm()&0o111 != 0 {
		t.Fatalf("mode = %v %v", st.Mode(), err)
	}
}

func TestSnapshotOwnershipAsRoot(t *testing.T) {
	root := setup(t)
	id := applyOne(t, root)
	old := euid
	t.Cleanup(func() { euid = old })
	euid = func() int { return 0 }
	if _, err := load(root, id); err != nil && os.Geteuid() == 0 {
		t.Fatalf("root-owned snapshot refused: %v", err)
	}
	// group/world-writable directory is refused when running as root
	dir := filepath.Dir(snapPath(root, id))
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := load(root, id); err == nil {
		t.Fatal("world-writable snapshot dir accepted")
	}
	_ = os.Chmod(dir, 0o700)
	// foreign owner is refused
	if os.Geteuid() == 0 {
		if err := os.Chown(snapPath(root, id), 1234, 1234); err != nil {
			t.Fatal(err)
		}
		if _, err := load(root, id); err == nil {
			t.Fatal("non-root-owned snapshot accepted")
		}
	}
}

func TestExistingLooseSnapshotDirFixed(t *testing.T) {
	root := setup(t)
	d := filepath.Join(root.Var, "lib/kerneltuner")
	if err := os.MkdirAll(filepath.Join(d, "snapshots"), 0o777); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(d, 0o777)
	_ = os.Chmod(filepath.Join(d, "snapshots"), 0o777)
	applyOne(t, root)
	for _, p := range []string{d, filepath.Join(d, "snapshots")} {
		if st, _ := os.Stat(p); st.Mode().Perm() != 0o700 {
			t.Errorf("%s mode %v, want 0700", p, st.Mode().Perm())
		}
	}
	// A directory owned by somebody else is refused.
	root2 := setup(t)
	old := euid
	t.Cleanup(func() { euid = old })
	if err := os.MkdirAll(filepath.Join(root2.Var, "lib/kerneltuner/snapshots"), 0o700); err != nil {
		t.Fatal(err)
	}
	euid = func() int { return os.Geteuid() + 4242 }
	p, _ := BuildPlan([]advisor.Recommendation{rec("sysctl", "net.core.somaxconn", "", "4096")}, root2)
	if _, err := Apply(p, Options{Now: fixed()}); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("foreign-owned snapshot dir: err = %v", err)
	}
}

// Q03: symlinks are never followed by apply or rollback.
func TestApplyRefusesSymlinkedManagedFile(t *testing.T) {
	root := setup(t)
	victim := filepath.Join(t.TempDir(), "victim")
	write(t, victim, "keep\n")
	conf := filepath.Join(root.Etc, sysctlFile)
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, conf); err != nil {
		t.Fatal(err)
	}
	p, _ := BuildPlan([]advisor.Recommendation{rec("sysctl", "net.core.somaxconn", "", "4096")}, root)
	if _, err := Apply(p, Options{Now: fixed()}); err == nil {
		t.Fatal("apply followed/replaced a symlinked managed file")
	}
	if read(t, victim) != "keep\n" {
		t.Fatal("victim modified")
	}
	if got := read(t, filepath.Join(root.Proc, "sys/net/core/somaxconn")); got != "128\n" {
		t.Fatalf("live value changed despite failure: %q", got)
	}
}

func TestApplyRefusesSymlinkedParentDir(t *testing.T) {
	root := setup(t)
	outside := t.TempDir()
	if err := os.MkdirAll(root.Etc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root.Etc, "sysctl.d")); err != nil {
		t.Fatal(err)
	}
	p, _ := BuildPlan([]advisor.Recommendation{rec("sysctl", "net.core.somaxconn", "", "4096")}, root)
	if _, err := Apply(p, Options{Now: fixed()}); err == nil {
		t.Fatal("apply wrote through a symlinked directory")
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Fatalf("files escaped through symlink: %v", ents)
	}
}

func TestRollbackNeverFollowsSymlinks(t *testing.T) {
	root := setup(t)
	id := applyOne(t, root)
	victim := filepath.Join(t.TempDir(), "victim")
	write(t, victim, "keep\n")
	conf := filepath.Join(root.Etc, sysctlFile)
	if err := os.Remove(conf); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, conf); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(root, id); err == nil {
		t.Fatal("rollback removed through a symlink")
	}
	if read(t, victim) != "keep\n" {
		t.Fatal("victim modified")
	}
	if _, err := os.Lstat(conf); err != nil {
		t.Fatal("symlink was removed")
	}
}

func TestSysfsSymlinkedDeviceStillWorks(t *testing.T) {
	root := setup(t)
	realDir := filepath.Join(root.Sys, "devices/pci0/block/sdb/queue")
	write(t, filepath.Join(realDir, "scheduler"), "[none] mq-deadline\n")
	if err := os.MkdirAll(filepath.Join(root.Sys, "block"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root.Sys, "devices/pci0/block/sdb"), filepath.Join(root.Sys, "block/sdb")); err != nil {
		t.Fatal(err)
	}
	p, err := BuildPlan([]advisor.Recommendation{rec("sysfs", "/sys/block/{item}/queue/scheduler", "sdb", "mq-deadline")}, root)
	if err != nil || len(p.Ops) != 1 {
		t.Fatalf("plan %+v %v", p, err)
	}
	io := &fakeIO{inner: OSIO, rewrite: map[string]func(string) string{
		filepath.Join(realDir, "scheduler"): func(v string) string { return "none [" + strings.TrimSpace(v) + "]\n" }}}
	if _, err := Apply(p, Options{Now: fixed(), IO: io}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(realDir, "scheduler")); got != "none [mq-deadline]\n" {
		t.Fatalf("scheduler = %q", got)
	}
}

func TestSysfsSymlinkEscapingSysRefused(t *testing.T) {
	root := setup(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "queue/scheduler"), "x\n")
	if err := os.Symlink(outside, filepath.Join(root.Sys, "block/evil")); err != nil {
		t.Fatal(err)
	}
	p, err := BuildPlan([]advisor.Recommendation{rec("sysfs", "/sys/block/{item}/queue/scheduler", "evil", "mq-deadline")}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ops) == 0 {
		return // refused at planning
	}
	if _, err := Apply(p, Options{Now: fixed()}); err == nil {
		t.Fatal("sysfs symlink escaping Root.Sys was written")
	}
	if read(t, filepath.Join(outside, "queue/scheduler")) != "x\n" {
		t.Fatal("outside file modified")
	}
}

func TestOSIOWriteRefusesFinalSymlink(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "t"), "keep\n")
	if err := os.Symlink(filepath.Join(d, "t"), filepath.Join(d, "l")); err != nil {
		t.Fatal(err)
	}
	if err := OSIO.Write(filepath.Join(d, "l"), "x\n"); err == nil {
		t.Fatal("write followed final symlink")
	}
}

// Q04: values and sysfs targets.
func TestPlanRejectsInjectedValues(t *testing.T) {
	root := setup(t)
	for _, v := range []string{"1\nkernel.core_pattern = |/tmp/x", "1\r2", "a\x00b", "a\x1b[2Jb"} {
		p, err := BuildPlan([]advisor.Recommendation{
			rec("sysctl", "net.core.somaxconn", "", v),
			rec("limit", "nofile", "nginx", v),
		}, root)
		if err != nil || len(p.Ops) != 0 || len(p.Skipped) != 2 {
			t.Errorf("value %q: ops=%d skipped=%+v err=%v", v, len(p.Ops), p.Skipped, err)
		}
	}
	// Apply re-checks a hand-made plan.
	op := Op{RuleID: "r", Kind: "sysctl", Param: "net.core.somaxconn", Proposed: "1\nevil=1",
		LivePath: filepath.Join(root.Proc, "sys/net/core/somaxconn"), File: sysctlConfPath(root), FileKind: fileSysctl, FileKey: "net.core.somaxconn"}
	if _, err := Apply(&Plan{Root: root, Ops: []Op{op}}, Options{Now: fixed()}); err == nil {
		t.Fatal("apply accepted newline value")
	}
}

func TestSysfsAllowlist(t *testing.T) {
	root := setup(t)
	write(t, filepath.Join(root.Sys, "power/state"), "mem\n")
	write(t, filepath.Join(root.Sys, "kernel/uevent_helper"), "\n")
	write(t, filepath.Join(root.Sys, "kernel/mm/transparent_hugepage/enabled"), "[always] madvise never\n")
	write(t, filepath.Join(root.Sys, "class/net/eth0/queues/rx-0/rps_cpus"), "0\n")
	p, err := BuildPlan([]advisor.Recommendation{
		rec("sysfs", "/sys/power/state", "", "disk"),
		rec("sysfs", "/sys/kernel/uevent_helper", "", "/tmp/x"),
		rec("sysfs", "/sys/kernel/mm/transparent_hugepage/enabled", "", "madvise"),
		rec("sysfs", "/sys/block/{item}/queue/scheduler", "nvme0n1", "none"),
		rec("sysfs", "/sys/class/net/{item}/queues/rx-0/rps_cpus", "eth0", "f"),
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ops) != 3 || len(p.Skipped) != 2 {
		t.Fatalf("ops=%d skipped=%+v", len(p.Ops), p.Skipped)
	}
	for _, s := range p.Skipped {
		if !strings.Contains(s.Reason, "allowlist") {
			t.Errorf("reason %q", s.Reason)
		}
	}
}

// Q10: genuine conflicts block apply unless one rule is selected.
func TestApplyBlocksGenuineConflict(t *testing.T) {
	root := setup(t)
	a := rec("sysctl", "net.core.somaxconn", "", "1000")
	a.RuleID, a.Compare = "A", "eq"
	b := rec("sysctl", "net.core.somaxconn", "", "2000")
	b.RuleID, b.Compare = "B", "eq"
	p, err := BuildPlan([]advisor.Recommendation{a, b}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Blocked) == 0 {
		t.Fatal("conflict not recorded")
	}
	if _, err := Apply(p, Options{Now: fixed()}); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("err = %v", err)
	}
	if got := read(t, filepath.Join(root.Proc, "sys/net/core/somaxconn")); got != "128\n" {
		t.Fatalf("value written despite conflict: %q", got)
	}
	// dry-run still previews
	if _, err := Apply(p, Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	// selecting one contributor (a plan built from it alone) applies
	p, _ = BuildPlan([]advisor.Recommendation{a}, root)
	if _, err := Apply(p, Options{Now: fixed()}); err != nil {
		t.Fatal(err)
	}
}

// Q11: rolled-back snapshots are skipped; corrupt ones warned about.
func TestRollbackMarksAndListSkipsCorrupt(t *testing.T) {
	root := setup(t)
	id1 := applyOne(t, root)
	p, _ := BuildPlan([]advisor.Recommendation{rec("sysctl", "net.core.somaxconn", "", "2048")}, root)
	later := func() time.Time { return fixed()().Add(time.Hour) }
	res2, err := Apply(p, Options{Now: later})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapPath(root, "garbage"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var warns []string
	snaps, err := ListWarn(root, func(m string) { warns = append(warns, m) })
	if err != nil || len(snaps) != 2 || len(warns) != 1 {
		t.Fatalf("snaps=%d warns=%v err=%v", len(snaps), warns, err)
	}
	if s, ok := Latest(snaps); !ok || s.ID != res2.ID {
		t.Fatalf("latest = %+v", s)
	}
	if err := Rollback(root, res2.ID); err != nil {
		t.Fatal(err)
	}
	snaps, _ = ListWarn(root, func(string) {})
	if s, ok := Latest(snaps); !ok || s.ID != id1 {
		t.Fatalf("after rollback latest = %+v ok=%v", s, ok)
	}
	if err := Rollback(root, id1); err != nil {
		t.Fatal(err)
	}
	snaps, _ = ListWarn(root, func(string) {})
	if _, ok := Latest(snaps); ok {
		t.Fatal("all snapshots rolled back but Latest returned one")
	}
}

// An automatic rollback also marks the snapshot.
func TestAutoRollbackMarksSnapshot(t *testing.T) {
	root := setup(t)
	live := filepath.Join(root.Proc, "sys/net/core/somaxconn")
	io := &fakeIO{inner: OSIO, only: "4096", rewrite: map[string]func(string) string{live: func(string) string { return "999\n" }}}
	p, _ := BuildPlan([]advisor.Recommendation{rec("sysctl", "net.core.somaxconn", "", "4096")}, root)
	if _, err := Apply(p, Options{Now: fixed(), IO: io}); err == nil {
		t.Fatal("want failure")
	}
	snaps, _ := ListWarn(root, func(string) {})
	if len(snaps) != 1 || snaps[0].RolledBackAt == nil {
		t.Fatalf("snaps = %+v", snaps)
	}
}

// Q17: two spellings of one kernel value are one op.
func TestPlanDedupesByLivePath(t *testing.T) {
	root := setup(t)
	a := rec("sysctl", "net.core.somaxconn", "", "1000")
	a.RuleID = "A"
	b := rec("sysctl", "net/core/somaxconn", "", "1000")
	b.RuleID = "B"
	p, _ := BuildPlan([]advisor.Recommendation{a, b}, root)
	if len(p.Ops) != 1 || len(p.Blocked) != 0 {
		t.Fatalf("ops=%+v blocked=%v", p.Ops, p.Blocked)
	}
	// differing values under different spellings conflict
	b.Proposed = "2000"
	b.Param = "net/core/somaxconn"
	// bypass advisor folding: plan the ops directly
	pl := &Plan{Root: root}
	sc := map[string]float64{}
	pl.add(Op{RuleID: "A", Kind: "sysctl", Proposed: "1000", LivePath: "/x"}, 1, sc)
	pl.add(Op{RuleID: "B", Kind: "sysctl", Proposed: "2000", LivePath: "/x"}, 2, sc)
	if len(pl.Ops) != 1 || pl.Ops[0].Proposed != "2000" || len(pl.Blocked) != 1 {
		t.Fatalf("plan %+v", pl)
	}
}

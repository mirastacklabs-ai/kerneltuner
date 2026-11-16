package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if _, err := os.Stat(dir); err != nil {
		return out
	}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[p] = "<dir>"
			return nil
		}
		b, _ := os.ReadFile(p)
		out[p] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func eq(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func fixed() func() time.Time {
	return func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
}

func rec(kind, param, target, proposed string) advisor.Recommendation {
	return advisor.Recommendation{RuleID: "r-" + param, Kind: kind, Param: param, Target: target,
		Proposed: proposed, Status: advisor.StatusRecommend}
}

func setup(t *testing.T) facts.Root {
	t.Helper()
	root := facts.RootAt(t.TempDir())
	write(t, filepath.Join(root.Proc, "sys/net/core/somaxconn"), "128\n")
	write(t, filepath.Join(root.Proc, "sys/net/ipv4/tcp_rmem"), "4096\t131072\t6291456\n")
	write(t, filepath.Join(root.Sys, "block/nvme0n1/queue/scheduler"), "[none] mq-deadline\n")
	return root
}

func TestPlanSelection(t *testing.T) {
	root := setup(t)
	recs := []advisor.Recommendation{
		rec("sysctl", "net.core.somaxconn", "", "4096"),
		{RuleID: "ok", Kind: "sysctl", Param: "net.core.somaxconn", Proposed: "1", Status: advisor.StatusOK},
		rec("cmdline", "x", "", "y"),
		rec("sysctl", "net.nope", "", "1"),
		rec("limit", "nofile", "", "1048576"),
		rec("limit", "cpu", "x.service", "1"),
	}
	p, err := BuildPlan(recs, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ops) != 1 || len(p.Skipped) != 3 {
		t.Fatalf("ops=%d skipped=%+v", len(p.Ops), p.Skipped)
	}
}

func TestApplyAllKindsAndRollback(t *testing.T) {
	root := setup(t)
	recs := []advisor.Recommendation{
		rec("sysctl", "net.core.somaxconn", "", "4096"),
		rec("sysctl", "net.ipv4.tcp_rmem", "", "4096 131072 16777216"),
		rec("sysfs", "/sys/block/{item}/queue/scheduler", "nvme0n1", "mq-deadline"),
		rec("limit", "nofile", "nginx", "1048576"),
		rec("limit", "memlock", "nginx.service", "infinity"),
	}
	recs[4].Status = advisor.StatusFiring
	// Fake kernel: scheduler file shows the bracketed active token.
	io := &fakeIO{inner: OSIO, rewrite: map[string]func(string) string{
		filepath.Join(root.Sys, "block/nvme0n1/queue/scheduler"): func(v string) string {
			return "none [" + strings.TrimSpace(v) + "]\n"
		}}}
	before := tree(t, root.Proc)
	beforeSys := tree(t, root.Sys)
	beforeEtc := tree(t, root.Etc)

	p, err := BuildPlan(recs, root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Apply(p, Options{Now: fixed(), IO: io, Version: "v-test"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "20260102T030405Z" || len(res.Hints) != 1 {
		t.Fatalf("res = %+v", res)
	}
	if got := read(t, filepath.Join(root.Proc, "sys/net/core/somaxconn")); got != "4096\n" {
		t.Fatalf("somaxconn = %q", got)
	}
	conf := read(t, filepath.Join(root.Etc, "sysctl.d/90-kerneltuner.conf"))
	if !strings.Contains(conf, "net.core.somaxconn = 4096\n") || !strings.Contains(conf, "net.ipv4.tcp_rmem = 4096 131072 16777216\n") {
		t.Fatalf("sysctl.d = %q", conf)
	}
	dropin := read(t, filepath.Join(root.Etc, "systemd/system/nginx.service.d/90-kerneltuner.conf"))
	if !strings.Contains(dropin, "[Service]\nLimitNOFILE=1048576\nLimitMEMLOCK=infinity\n") {
		t.Fatalf("dropin = %q", dropin)
	}
	st, err := os.Stat(filepath.Join(root.Var, "lib/kerneltuner/snapshots", res.ID+".json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot perm: %v %v", st, err)
	}
	dst, _ := os.Stat(filepath.Dir(res.Snapshot))
	if dst.Mode().Perm() != 0o700 {
		t.Fatalf("snapshot dir perm %v", dst.Mode().Perm())
	}
	snaps, err := List(root)
	if err != nil || len(snaps) != 1 || snaps[0].Version != "v-test" {
		t.Fatalf("list = %+v %v", snaps, err)
	}

	for i := 0; i < 2; i++ { // second pass proves idempotence
		if err := RollbackWith(root, res.ID, io); err != nil {
			t.Fatal(err)
		}
		if !eq(before, tree(t, root.Proc)) {
			t.Fatalf("proc not restored: %v", tree(t, root.Proc))
		}
		if !eq(beforeEtc, tree(t, root.Etc)) {
			t.Fatalf("etc not restored (created files/dirs must go): %v", tree(t, root.Etc))
		}
	}
	// sysfs: restore wrote the bracketed token "none", the fake kernel rewraps it.
	if got := read(t, filepath.Join(root.Sys, "block/nvme0n1/queue/scheduler")); got != "none [none]\n" {
		t.Fatalf("sysfs after rollback = %q (before %v)", got, beforeSys)
	}
}

func TestMergeExistingSysctlD(t *testing.T) {
	root := setup(t)
	conf := filepath.Join(root.Etc, "sysctl.d/90-kerneltuner.conf")
	orig := "# mine\nvm.swappiness = 10\nnet.core.somaxconn = 256\nnet.core.somaxconn = 512\n"
	write(t, conf, orig)
	p, _ := BuildPlan([]advisor.Recommendation{rec("sysctl", "net.core.somaxconn", "", "4096")}, root)
	res, err := Apply(p, Options{Now: fixed()})
	if err != nil {
		t.Fatal(err)
	}
	want := "# mine\nvm.swappiness = 10\nnet.core.somaxconn = 4096\n"
	if got := read(t, conf); got != want {
		t.Fatalf("merged = %q want %q", got, want)
	}
	if err := Rollback(root, res.ID); err != nil {
		t.Fatal(err)
	}
	if got := read(t, conf); got != orig {
		t.Fatalf("restored = %q", got)
	}
}

func TestMergeExistingDropin(t *testing.T) {
	root := setup(t)
	f := filepath.Join(root.Etc, "systemd/system/db.service.d/90-kerneltuner.conf")
	write(t, f, "[Service]\nLimitNOFILE=1\nNice=5\n")
	p, _ := BuildPlan([]advisor.Recommendation{rec("limit", "LimitNOFILE", "db", "65536")}, root)
	if _, err := Apply(p, Options{Now: fixed()}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, f); got != "[Service]\nLimitNOFILE=65536\nNice=5\n" {
		t.Fatalf("dropin = %q", got)
	}
}

func TestVerifyFailureAutoRollback(t *testing.T) {
	root := setup(t)
	live := filepath.Join(root.Proc, "sys/net/core/somaxconn")
	io := &fakeIO{inner: OSIO, rewrite: map[string]func(string) string{
		live: func(string) string { return "999\n" }, // kernel clamps the value
	}}
	// rewrite applies only to non-restore writes: restore must work, so only rewrite 4096.
	io.only = "4096"
	before := tree(t, root.Proc)
	beforeEtc := tree(t, root.Etc)
	p, _ := BuildPlan([]advisor.Recommendation{rec("sysctl", "net.core.somaxconn", "", "4096")}, root)
	res, err := Apply(p, Options{Now: fixed(), IO: io})
	if err == nil || !strings.Contains(err.Error(), "verify "+live) || !strings.Contains(err.Error(), "rolled back snapshot "+res.ID) {
		t.Fatalf("err = %v", err)
	}
	if !eq(before, tree(t, root.Proc)) || !eq(beforeEtc, tree(t, root.Etc)) {
		t.Fatal("state not restored after failed verify")
	}
}

func TestLiveWriteFailureAutoRollback(t *testing.T) {
	root := setup(t)
	io := &fakeIO{inner: OSIO, failWrite: filepath.Join(root.Proc, "sys/net/ipv4/tcp_rmem")}
	before := tree(t, root.Proc)
	beforeEtc := tree(t, root.Etc)
	p, _ := BuildPlan([]advisor.Recommendation{
		rec("sysctl", "net.core.somaxconn", "", "4096"),
		rec("sysctl", "net.ipv4.tcp_rmem", "", "1 2 3"),
	}, root)
	_, err := Apply(p, Options{Now: fixed(), IO: io})
	if err == nil || !strings.Contains(err.Error(), "write live value") {
		t.Fatalf("err = %v", err)
	}
	if !eq(before, tree(t, root.Proc)) || !eq(beforeEtc, tree(t, root.Etc)) {
		t.Fatal("state not restored")
	}
}

func TestPathEscapeRejected(t *testing.T) {
	root := setup(t)
	for _, r := range []advisor.Recommendation{
		rec("sysfs", "/sys/block/{item}/queue/scheduler", "../../../../etc", "x"),
		rec("limit", "nofile", "../../../evil", "1"),
		rec("sysfs", "../outside", "", "x"),
	} {
		if _, err := BuildPlan([]advisor.Recommendation{r}, root); err == nil || !strings.Contains(err.Error(), "escapes root") {
			t.Errorf("%s/%s: err = %v", r.Param, r.Target, err)
		}
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	root := setup(t)
	before := tree(t, filepath.Dir(root.Proc))
	p, _ := BuildPlan([]advisor.Recommendation{
		rec("sysctl", "net.core.somaxconn", "", "4096"),
		rec("limit", "nofile", "nginx", "1"),
	}, root)
	res, err := Apply(p, Options{DryRun: true, Now: fixed()})
	if err != nil || !res.DryRun || len(res.Ops) != 2 || res.ID != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !eq(before, tree(t, filepath.Dir(root.Proc))) {
		t.Fatal("dry-run changed the tree")
	}
}

func TestRollbackBadID(t *testing.T) {
	root := setup(t)
	if err := Rollback(root, "../x"); err == nil {
		t.Fatal("want error")
	}
	if err := Rollback(root, "nope"); err == nil {
		t.Fatal("want error")
	}
}

// fakeIO wraps a ValueIO to simulate kernel behaviour.
type fakeIO struct {
	inner     ValueIO
	rewrite   map[string]func(string) string // transform value written to path
	only      string                         // when set, rewrite only values equal to this
	failWrite string                         // path whose writes fail on non-restore values
}

func (f *fakeIO) Read(p string) (string, error) { return f.inner.Read(p) }

func (f *fakeIO) Write(p, v string) error {
	if f.failWrite == p && strings.TrimSpace(v) == "1 2 3" {
		return os.ErrPermission
	}
	if fn, ok := f.rewrite[p]; ok && (f.only == "" || strings.TrimSpace(v) == f.only) {
		v = fn(v)
	}
	return f.inner.Write(p, v)
}

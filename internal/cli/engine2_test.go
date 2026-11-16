package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixturetest"
)

// validate --rules-dir on a missing directory, or on a directory holding a
// "rules" subdirectory (what `cp -r rules DEST` makes), used to report
// "OK: 0 rules".
func TestValidateRulesDirWithoutPacksFails(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{filepath.Join(root, "missing"), root} {
		_, _, err := runIn(t, root, dir, "validate")
		if err == nil || !strings.Contains(err.Error(), "no rule packs found") || !strings.Contains(err.Error(), "--rules-dir") {
			t.Errorf("validate --rules-dir %s: %v", dir, err)
		}
	}
	nested := t.TempDir()
	if err := os.MkdirAll(filepath.Join(nested, "rules", "kernel"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runIn(t, root, nested, "validate"); err == nil || !strings.Contains(err.Error(), `"rules" subdirectory`) {
		t.Errorf("nested copy: %v", err)
	}
}

func TestValidateRulesDirBadSourcesNamesFileAndLine(t *testing.T) {
	dir := t.TempDir()
	pack := filepath.Join(dir, "apps", "x")
	if err := os.MkdirAll(pack, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, "sources.yaml"), []byte("- url: https://a\n  bogus: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runIn(t, t.TempDir(), dir, "validate")
	if err == nil || !strings.Contains(err.Error(), "apps/x/sources.yaml:2:") {
		t.Errorf("error = %v", err)
	}
}

// The fixture round trip also covers the mount table, kernel config,
// oom_score_adj and nproc/core limits. Remote mount devices are redacted.
func TestCaptureRoundTripEngineFacts2(t *testing.T) {
	src, _, _ := fixturetest.Load(t, "engine-facts-2")
	want, err := facts.Collect(src)
	if err != nil {
		t.Fatal(err)
	}
	yml := runCapture(t, "--root", filepath.Dir(src.Proc), "--fixture")
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "rt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "rt", "fixture.yaml"), yml, 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := fixture.ParseIn(base, "rt")
	if err != nil {
		t.Fatal(err)
	}
	dst, _, _, err := fixture.MaterializeSpec(t.TempDir(), spec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := facts.Collect(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Mounts) != len(want.Mounts) {
		t.Fatalf("mounts: %+v vs %+v", got.Mounts, want.Mounts)
	}
	for i, m := range want.Mounts {
		g := got.Mounts[i]
		if m.FSType == "nfs4" {
			if g.Device != "[remote]" {
				t.Errorf("remote device not redacted: %+v", g)
			}
			g.Device = m.Device
		}
		if g.MountPoint != m.MountPoint || g.FSType != m.FSType || g.Disk != m.Disk || g.Device != m.Device || strings.Join(g.Options, ",") != strings.Join(m.Options, ",") {
			t.Errorf("mount %d: got %+v want %+v", i, g, m)
		}
	}
	for k, v := range want.Kernel.Config {
		if got.Kernel.Config[k] != v {
			t.Errorf("config %s = %q, want %q", k, got.Kernel.Config[k], v)
		}
	}
	for i, p := range want.Procs {
		if q := got.Procs[i]; q.OOMScoreAdj != p.OOMScoreAdj || q.NProcSoft != p.NProcSoft || q.NProcHard != p.NProcHard || q.CoreSoft != p.CoreSoft {
			t.Errorf("proc %d: got %+v want %+v", p.PID, q, p)
		}
	}
}

func TestSanitizeMounts(t *testing.T) {
	in := "srv.corp.example:/export /srv nfs4 rw,addr=10.1.2.3,clientaddr=10.1.2.4,vers=4.2 0 0\n" +
		"//nas/share /mnt/s cifs rw,username=bob,password=x,vers=3.0 0 0\n" +
		"/dev/sda1 /data ext4 rw,relatime 0 0\n"
	out := sanitizeMounts(in)
	for _, bad := range []string{"srv.corp", "10.1.2", "bob", "password", "nas"} {
		if strings.Contains(out, bad) {
			t.Errorf("leaked %q:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "[remote] /srv nfs4 rw,vers=4.2 0 0") || !strings.Contains(out, "/dev/sda1 /data ext4 rw,relatime 0 0") {
		t.Errorf("sanitized:\n%s", out)
	}
}

func TestCheckoutFixturesNoUpwardWalk(t *testing.T) {
	co := t.TempDir()
	for _, d := range []string{"rules", filepath.Join("testdata", "fixtures"), "sub"} {
		if err := os.MkdirAll(filepath.Join(co, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	want := filepath.Join(co, "testdata", "fixtures")
	if got := checkoutFixtures("", co); got != want {
		t.Errorf("cwd checkout = %q, want %q", got, want)
	}
	if got := checkoutFixtures(filepath.Join(co, "rules"), t.TempDir()); got != want {
		t.Errorf("rules-dir parent = %q, want %q", got, want)
	}
	// A subdirectory of a checkout must not find the checkout above it.
	if got := checkoutFixtures("", filepath.Join(co, "sub")); got != "" {
		t.Errorf("subdir found %q", got)
	}
	// testdata without rules/ is not a checkout.
	bare := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bare, "testdata", "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := checkoutFixtures("", bare); got != "" {
		t.Errorf("no rules/: found %q", got)
	}
}

func TestValidateMissingFixturesIsError(t *testing.T) {
	old := fixturesDirFn
	fixturesDirFn = func() (string, error) { return "", os.ErrNotExist }
	defer func() { fixturesDirFn = old }()
	t.Chdir(t.TempDir()) // not a checkout

	packs := t.TempDir()
	if _, err := resolveFixtures(&globalOpts{RulesDir: packs}, false); err == nil || !strings.Contains(err.Error(), "--skip-golden") || !strings.Contains(err.Error(), "--fixtures-dir") {
		t.Errorf("resolveFixtures err = %v", err)
	}
	if d, err := resolveFixtures(&globalOpts{RulesDir: packs}, true); err != nil || d != "" {
		t.Errorf("skip: %q %v", d, err)
	}
}

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// R-Q10: magic /proc links cannot be reached through a rule path.
func TestResolveUnderRejectsMagicLinks(t *testing.T) {
	r := facts.RootAt("/fake")
	for _, p := range []string{"/proc/self/root/etc/shadow", "/proc/1/cwd/x", "/proc/123/exe", "/proc/thread-self/root", "/proc/self/../self/root/x", "/proc/42/root"} {
		if got, ok := ResolveUnder(r, p); ok {
			t.Errorf("ResolveUnder(%q) = %q, want refusal", p, got)
		}
	}
	if got, ok := ResolveUnder(r, "/proc/self/status"); !ok || got != "/fake/proc/self/status" {
		t.Errorf("benign path refused: %q %v", got, ok)
	}
}

// R-Q11: oversized expressions are refused.
func TestCompileCapsExpressionSize(t *testing.T) {
	big := "1" + strings.Repeat(" + 1", maxExprNodes)
	if _, err := Compile(big); err == nil {
		t.Fatal("expression with more than maxExprNodes nodes compiled")
	}
	if _, err := Compile("1 + 1"); err != nil {
		t.Fatal(err)
	}
}

// R-Q11: rules-dir loading skips symlinks, non-regular files and big files.
func TestLoadSkipsUnsafeRuleFiles(t *testing.T) {
	good := "- id: a.b\n  title: T\n"
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "kernel/x"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "kernel/x/ok.yaml"), []byte(good), 0o644))
	secret := filepath.Join(t.TempDir(), "secret.yaml")
	must(os.WriteFile(secret, []byte("- id: from.symlink\n  title: S\n"), 0o644))
	must(os.Symlink(secret, filepath.Join(dir, "kernel/x/link.yaml")))
	must(os.WriteFile(filepath.Join(dir, "kernel/x/big.yaml"), []byte("- id: big.one\n  title: "+strings.Repeat("a", maxRuleFile)+"\n"), 0o644))
	rs, _, _, err := Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	if len(ids) != 1 || ids[0] != "a.b" {
		t.Fatalf("loaded %v, want only a.b", ids)
	}
	// non-regular entries from any fs.FS are skipped too
	m := fstest.MapFS{"kernel/x/dev.yaml": {Data: []byte(good), Mode: os.ModeNamedPipe}}
	if rs, _, _, err := Load(m); err != nil || len(rs) != 0 {
		t.Fatalf("pipe loaded: %v %v", rs, err)
	}
}

package fixture

import (
	"os"
	"path/filepath"
	"testing"
)

// R-Q02: symlinks must stay inside the fixture and nothing is created through them.
func TestMaterializeRejectsUnsafeSymlinks(t *testing.T) {
	for name, target := range map[string]string{"absolute": "/etc/passwd", "escaping": "../../../etc/passwd", "deep-escape": "a/../../../x"} {
		s := &Spec{Symlinks: map[string]string{"sys/link": target}}
		if _, _, _, err := MaterializeSpec(t.TempDir(), s); err == nil {
			t.Errorf("%s target %q accepted", name, target)
		}
	}
	s := &Spec{Symlinks: map[string]string{"sys/ok": "../proc/x"}}
	if _, _, _, err := MaterializeSpec(t.TempDir(), s); err != nil {
		t.Errorf("relative in-root link rejected: %v", err)
	}
}

func TestMaterializeNeverWritesThroughSymlink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "proc")); err != nil {
		t.Fatal(err)
	}
	s := &Spec{Files: map[string]string{"proc/sys/kernel/osrelease": "6.1\n"}}
	if _, _, _, err := MaterializeSpec(dir, s); err == nil {
		t.Fatal("wrote through a symlinked directory")
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Fatalf("files escaped: %v", ents)
	}
	// final component symlink
	dir2 := t.TempDir()
	victim := filepath.Join(outside, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir2, "proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir2, "proc/f")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := MaterializeSpec(dir2, &Spec{Files: map[string]string{"proc/f": "x"}}); err == nil {
		t.Fatal("wrote through a file symlink")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("victim = %q", b)
	}
}

func TestFixturesDirNeverWalksUp(t *testing.T) {
	t.Setenv(EnvDir, "")
	old := defaultDir
	defaultDir = ""
	t.Cleanup(func() { defaultDir = old })
	if d, err := FixturesDir(); err == nil {
		t.Fatalf("FixturesDir found %q without flag, env or test default", d)
	}
	t.Setenv(EnvDir, "/x/y")
	if d, _ := FixturesDir(); d != "/x/y" {
		t.Fatalf("env ignored: %q", d)
	}
}

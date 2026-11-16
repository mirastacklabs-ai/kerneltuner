package fixture

import (
	"os"
	"path/filepath"
	"testing"
)

// Q06: chained links must not reach outside the fixture root. Real symlinks.
func TestMaterializeRejectsChainedLinkEscape(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// a/b/c/d points at the root; x then climbs out through it.
	s := &Spec{Symlinks: map[string]string{
		"a/b/c/d": "../../..",
		"x":       "a/b/c/d/../../../../etc",
	}}
	if _, _, _, err := MaterializeSpec(filepath.Join(parent, "root"), s); err == nil {
		t.Fatal("chained-link escape accepted")
	}
}

// The Stat check catches an escape that passes the lexical check: k/up is a
// real link to the root, so k/up/up2 is really created at root/up2 and its
// ../../etc leaves the root.
func TestMaterializeStatCatchesEscapeThroughLink(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Spec{Symlinks: map[string]string{"k/up": "..", "k/up/up2": "../../etc"}}
	if _, _, _, err := MaterializeSpec(filepath.Join(parent, "root"), s); err == nil {
		t.Fatal("escape through a link accepted")
	}
}

func TestMaterializeAllowsDanglingAndInRootLinks(t *testing.T) {
	s := &Spec{
		Files:    map[string]string{"proc/x": "1\n"},
		Symlinks: map[string]string{"sys/dangling": "../nothing/here", "sys/ok": "../proc/x"},
	}
	if _, _, _, err := MaterializeSpec(t.TempDir(), s); err != nil {
		t.Fatalf("legitimate links rejected: %v", err)
	}
}

package fixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFix(t *testing.T, base, name, body string) {
	t.Helper()
	d := filepath.Join(base, name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "fixture.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMinimal(t *testing.T) {
	root, values, rates := Load(t, "minimal")
	b, err := os.ReadFile(filepath.Join(root.Proc, "sys/kernel/osrelease"))
	if err != nil || !strings.HasPrefix(string(b), "6.8") {
		t.Fatalf("osrelease = %q, %v", b, err)
	}
	if values["conntrack.count"] != 1200 || rates["TcpExt.ListenOverflows"] != 3.2 {
		t.Fatalf("values=%v rates=%v", values, rates)
	}
}

func TestExtendsAndSymlinks(t *testing.T) {
	base := t.TempDir()
	writeFix(t, base, "a", "files:\n  proc/x: \"a\\n\"\n  proc/y: \"a\\n\"\nvalues: {m: 1}\n")
	writeFix(t, base, "b", "extends: a\nfiles:\n  proc/y: \"b\\n\"\nsymlinks:\n  sys/link: ../proc/y\nvalues: {n: 2}\n")
	s, err := ParseIn(base, "b")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root, values, _, err := MaterializeSpec(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root.Proc, "x")); string(b) != "a\n" {
		t.Errorf("x = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root.Proc, "y")); string(b) != "b\n" {
		t.Errorf("y = %q", b)
	}
	if b, err := os.ReadFile(filepath.Join(root.Sys, "link")); err != nil || string(b) != "b\n" {
		t.Errorf("symlink read = %q, %v", b, err)
	}
	if values["m"] != 1 || values["n"] != 2 {
		t.Errorf("values = %v", values)
	}
}

func TestExtendsCycle(t *testing.T) {
	base := t.TempDir()
	writeFix(t, base, "a", "extends: b\n")
	writeFix(t, base, "b", "extends: a\n")
	if _, err := ParseIn(base, "a"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("err = %v", err)
	}
}

func TestPathEscape(t *testing.T) {
	base := t.TempDir()
	writeFix(t, base, "e", "files:\n  ../evil: x\n")
	s, err := ParseIn(base, "e")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := MaterializeSpec(t.TempDir(), s); err == nil {
		t.Fatal("expected escape error")
	}
}

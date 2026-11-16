package facts

import (
	"path/filepath"
	"strings"
	"testing"
)

// R-Q09: a hostile release cannot select another file, and big configs are capped.
func TestKconfigReleaseTraversalAndCap(t *testing.T) {
	dir := t.TempDir()
	r := RootAt(dir)
	put(t, filepath.Join(dir, "boot/config-6.1.0"), "CONFIG_BPF=y\n")
	put(t, filepath.Join(dir, "x"), "CONFIG_EVIL=y\n")
	f := &Facts{}
	f.Kernel.Release = "6.1.0"
	collectKconfig(r, f)
	if f.Kernel.Config["CONFIG_BPF"] != "y" {
		t.Fatalf("config = %v", f.Kernel.Config)
	}
	// release "../x" would read <root>/boot/config-../x == <root>/x
	put(t, filepath.Join(dir, "boot/config-"), "")
	for _, rel := range []string{"../x", "a/b", `..\x`} {
		f := &Facts{}
		f.Kernel.Release = rel
		collectKconfig(r, f)
		if len(f.Kernel.Config) != 0 {
			t.Errorf("release %q read a config: %v", rel, f.Kernel.Config)
		}
	}
	// a plain config larger than the cap is truncated, not slurped whole
	big := "CONFIG_A=y\n" + strings.Repeat("#"+strings.Repeat("x", 1023)+"\n", 17<<10)
	put(t, filepath.Join(dir, "boot/config-9.9"), big+"CONFIG_LATE=y\n")
	f = &Facts{}
	f.Kernel.Release = "9.9"
	collectKconfig(r, f)
	if f.Kernel.Config["CONFIG_A"] != "y" || f.Kernel.Config["CONFIG_LATE"] != "" {
		t.Errorf("cap not applied: A=%q LATE=%q", f.Kernel.Config["CONFIG_A"], f.Kernel.Config["CONFIG_LATE"])
	}
}

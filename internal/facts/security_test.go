package facts

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func put(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Q01: only units directly under system.slice count.
func TestUnitFromCgroupOnlySystemSlice(t *testing.T) {
	cases := map[string]string{
		"0::/system.slice/nginx.service\n":                                       "nginx.service",
		"0::/system.slice/system-getty.slice/getty@tty1.service\n":               "getty@tty1.service",
		"0::/system.slice/nginx.service/worker\n":                                "nginx.service",
		"0::/user.slice/user-1000.slice/user@1000.service/app.slice/x.service\n": "",
		"0::/user.slice/user-1000.slice/user@1000.service/evil.service\n":        "",
		"0::/system.slice/foo.service/evil.service\n":                            "foo.service",
		"0::/system.slice/docker-abc.scope/evil.service\n":                       "",
		"0::/system.slice/bad name.service\n":                                    "",
		"0::/system.slice/$(id).service\n":                                       "",
		"0::/system.slice/a;b.service\n":                                         "",
		"0::/foo/system.slice/nginx.service\n":                                   "",
		"0::/system.slice/user@1000.service\n":                                   "",
		"0::/system.slice/..service\n":                                           "",
		"1:name=systemd:/system.slice/sshd.service\n0::/\n":                      "sshd.service",
	}
	for cg, want := range cases {
		if got := unitFromCgroup(cg); got != want {
			t.Errorf("unitFromCgroup(%q) = %q, want %q", cg, got, want)
		}
	}
}

func TestSafeTextStripsControls(t *testing.T) {
	in := "a\x1b[31mb\nc\rd\x7fe\u009bf\xff"
	got := SafeText(in)
	for _, r := range got {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			t.Fatalf("control character survived: %q", got)
		}
	}
	if SafeText("plain text-1.2") != "plain text-1.2" {
		t.Fatal("clean text changed")
	}
}

func TestCollectProcsSanitizesAndBounds(t *testing.T) {
	r := RootAt(t.TempDir())
	put(t, filepath.Join(r.Proc, "1/comm"), "ev\x1b[2Jil\n")
	put(t, filepath.Join(r.Proc, "1/cmdline"), "/bin/x\x00--a\x1b]0;pwn\x07\x00")
	put(t, filepath.Join(r.Proc, "1/cgroup"), "0::/system.slice/x.service\n")
	// a huge cmdline is truncated at the cap
	put(t, filepath.Join(r.Proc, "2/comm"), "big\n")
	put(t, filepath.Join(r.Proc, "2/cmdline"), strings.Repeat("A", 1<<20))
	f := &Facts{}
	collectProcs(r, f)
	if len(f.Procs) != 2 {
		t.Fatalf("procs = %d", len(f.Procs))
	}
	if strings.ContainsAny(f.Procs[0].Comm, "\x1b") || strings.ContainsAny(strings.Join(f.Procs[0].Cmdline, ""), "\x1b\x07") {
		t.Errorf("control characters kept: %q %q", f.Procs[0].Comm, f.Procs[0].Cmdline)
	}
	if n := len(f.Procs[1].Cmdline[0]); n > procFileLimit {
		t.Errorf("cmdline read %d bytes, cap %d", n, procFileLimit)
	}
}

func TestCollectProcsCapsCount(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	r := RootAt(t.TempDir())
	if err := os.MkdirAll(r.Proc, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= maxProcs+50; i++ {
		d := filepath.Join(r.Proc, strconv.Itoa(i))
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "comm"), []byte("p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &Facts{}
	collectProcs(r, f)
	if len(f.Procs) != maxProcs {
		t.Fatalf("procs = %d, want cap %d", len(f.Procs), maxProcs)
	}
}

// Q15: write-triggering and secret sysctls are never read.
func TestCollectSysctlSkipsDangerous(t *testing.T) {
	r := RootAt(t.TempDir())
	base := filepath.Join(r.Proc, "sys")
	put(t, filepath.Join(base, "vm/stat_refresh"), "1\n")
	put(t, filepath.Join(base, "net/ipv4/tcp_fastopen_key"), "secret\n")
	put(t, filepath.Join(base, "kernel/hostname"), "box\n")
	put(t, filepath.Join(base, "vm/swappiness"), "60\n")
	f := &Facts{Sysctl: map[string]string{}}
	collectSysctl(r, f)
	for _, bad := range []string{"vm.stat_refresh", "net.ipv4.tcp_fastopen_key", "kernel.hostname"} {
		if _, ok := f.Sysctl[bad]; ok {
			t.Errorf("%s was read", bad)
		}
	}
	if f.Sysctl["vm.swappiness"] != "60" {
		t.Errorf("swappiness = %q", f.Sysctl["vm.swappiness"])
	}
}

// Q16: huge CPU ranges are capped.
func TestParseCPUListCapsRange(t *testing.T) {
	if got := parseCPUList("0-3,8"); len(got) != 5 {
		t.Fatalf("got %v", got)
	}
	if got := parseCPUList("0-4000000000"); len(got) != 0 {
		t.Fatalf("huge range expanded to %d entries", len(got))
	}
	if got := parseCPUList("0-65535"); len(got) != 65536 {
		t.Fatalf("max range = %d", len(got))
	}
}

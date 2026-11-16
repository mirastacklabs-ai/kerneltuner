package rules

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
)

func env2(t *testing.T) Env {
	t.Helper()
	dir := t.TempDir()
	for rel, s := range map[string]string{
		"proc/sys/fs/inotify/max_user_watches":  "524288\n",
		"proc/sys/net/bridge/x-nf":              "1\n",
		"proc/sys/kernel/cap_last_cap":          " 40 \n",
		"proc/sys/net/ipv4/tcp_rmem":            "4096\t131072\t6291456\n",
		"proc/sys/net/ipv4/conf/eth0/rp_filter": "2\n",
	} {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &facts.Facts{
		Sysctl: map[string]string{"vm.swappiness": "10"},
		Kernel: facts.Kernel{Config: map[string]string{"CONFIG_BPF_JIT": "y", "CONFIG_X": "n"}},
		Mounts: []facts.Mount{
			{Device: "/dev/sda1", MountPoint: "/", FSType: "ext4", Options: []string{"rw", "relatime"}, Disk: "sda"},
			{Device: "fs:/e", MountPoint: "/srv", FSType: "nfs4", Options: []string{"rw", "vers=4.2"}},
			{Device: "/dev/nvme0n1p1", MountPoint: "/srv/db", FSType: "xfs", Options: []string{"noatime"}, Disk: "nvme0n1"},
		},
	}
	w := &workload.Workload{Apps: []workload.App{{Name: "redis", NoFileSoft: math.MaxUint64}}}
	return NewEnv(EnvInput{Facts: f, Workload: w, Root: facts.RootAt(dir)}, float64(math.MaxUint64), nil)
}

func TestSysctlHelpersFallBackToProc(t *testing.T) {
	e := env2(t)
	cases := []struct {
		src  string
		want any
	}{
		{`sysctl("vm.swappiness")`, "10"},
		{`sysctl("fs.inotify.max_user_watches")`, "524288"},
		{`sysctlNum("fs.inotify.max_user_watches")`, float64(524288)},
		{`sysctlNum("net.bridge.x-nf")`, float64(1)},
		{`sysctl("kernel.cap_last_cap")`, "40"},
		{`sysctl("net.ipv4.tcp_rmem")`, "4096 131072 6291456"},
		{`sysctl("net.ipv4.conf.eth0.rp_filter")`, "2"},
		{`sysctl("does.not.exist")`, ""},
		{`sysctlNum("does.not.exist")`, float64(0)},
		{`sysctl("../etc/passwd")`, ""},
	}
	for _, c := range cases {
		got, err := Eval(c.src, e)
		if err != nil || got != c.want {
			t.Errorf("%s = %v (%v), want %v", c.src, got, err, c.want)
		}
	}
}

func TestMountAndKconfigHelpers(t *testing.T) {
	e := env2(t)
	cases := []struct {
		src  string
		want any
	}{
		{`fsType("/home")`, "ext4"},
		{`fsType("/srv/x")`, "nfs4"},
		{`fsType("/srv/db/data")`, "xfs"},
		{`mountOf("/srv/db/data").MountPoint`, "/srv/db"},
		{`mountOf("/srv/db/data").Device`, "/dev/nvme0n1p1"},
		{`mountHasOpt("/srv/db", "noatime")`, true},
		{`mountHasOpt("/", "noatime")`, false},
		{`mountHasOpt("/srv", "vers")`, true},
		{`mountHasOpt("/srv", "vers=4.2")`, true},
		{`blockOf("/srv/db/x")`, "nvme0n1"},
		{`blockOf("/srv/x")`, ""},
		{`kconfig("CONFIG_BPF_JIT") == "y"`, true},
		{`kconfig("CONFIG_X")`, "n"},
		{`kconfig("CONFIG_MISSING")`, ""},
	}
	for _, c := range cases {
		got, err := Eval(c.src, e)
		if err != nil || got != c.want {
			t.Errorf("%s = %v (%v), want %v", c.src, got, err, c.want)
		}
	}
	// No mount table: zero Mount, empty answers, no error.
	empty := NewEnv(EnvInput{}, nil, nil)
	for src, want := range map[string]any{`fsType("/")`: "", `mountHasOpt("/", "noatime")`: false, `blockOf("/")`: "", `kconfig("CONFIG_A")`: ""} {
		if got, err := Eval(src, empty); err != nil || got != want {
			t.Errorf("%s = %v (%v), want %v", src, got, err, want)
		}
	}
}

func TestUnlimited(t *testing.T) {
	e := env2(t)
	cases := []struct {
		src  string
		want any
	}{
		{`unlimited`, uint64(math.MaxUint64)},
		{`isUnlimited(unlimited)`, true},
		{`isUnlimited(current)`, true}, // float64(MaxUint64), as the limit kind provides
		{`current == unlimited`, true},
		{`isUnlimited(workload.Apps[0].NoFileSoft)`, true},
		{`workload.Apps[0].NoFileSoft == unlimited`, true},
		{`isUnlimited(1048576)`, false},
		{`isUnlimited(0)`, false},
		{`isUnlimited(-1)`, false},
		{`isUnlimited("unlimited")`, true},
		{`isUnlimited("infinity")`, true},
		{`isUnlimited("65536")`, false},
		{`isUnlimited(9223372036854775807)`, false},
		{`unlimited > 1e18`, true},
	}
	for _, c := range cases {
		got, err := Eval(c.src, e)
		if err != nil || got != c.want {
			t.Errorf("%s = %v (%v), want %v", c.src, got, err, c.want)
		}
	}
	// A real finite value is not unlimited.
	fin := NewEnv(EnvInput{}, float64(1<<40), nil)
	if got, err := Eval(`isUnlimited(current)`, fin); err != nil || got != false {
		t.Errorf("finite current: %v %v", got, err)
	}
}

func TestCheckPacks(t *testing.T) {
	if err := CheckPacks(fstest.MapFS{}); err == nil || !strings.Contains(err.Error(), "no rule packs found") {
		t.Errorf("empty dir: %v", err)
	}
	// A copy made with `cp -r rules DEST` nests the packs under DEST/rules.
	nested := fstest.MapFS{"rules/kernel/network/a.yaml": {Data: []byte("[]")}}
	err := CheckPacks(nested)
	if err == nil || !strings.Contains(err.Error(), `"rules" subdirectory`) {
		t.Errorf("nested copy must hint at the subdirectory: %v", err)
	}
	// An area directory without rules is fine.
	if err := CheckPacks(fstest.MapFS{"kernel/network/a.yaml": {Data: []byte("[]")}}); err != nil {
		t.Errorf("empty pack: %v", err)
	}
}

func TestSourcesYAMLErrorsNameFileAndLine(t *testing.T) {
	bad := []byte("- url: https://a\n  project: p\n- url: https://b\n  bogus: 1\n")
	_, _, _, err := Load(fstest.MapFS{"apps/x/sources.yaml": {Data: bad}})
	if err == nil || !strings.Contains(err.Error(), "apps/x/sources.yaml:4: field bogus not found") {
		t.Errorf("unknown field: %v", err)
	}
	_, _, _, err = Load(fstest.MapFS{"apps/x/sources.yaml": {Data: []byte("- url: [unclosed\n")}})
	if err == nil || !strings.Contains(err.Error(), "apps/x/sources.yaml:") || strings.Contains(err.Error(), "yaml: yaml") {
		t.Errorf("syntax error: %v", err)
	}
	_, _, _, err = Load(fstest.MapFS{"apps/x/sources.yaml": {Data: []byte("url: https://a\n")}})
	if err == nil || !strings.Contains(err.Error(), "apps/x/sources.yaml:1:") {
		t.Errorf("wrong shape: %v", err)
	}
	_, _, _, err = Load(fstest.MapFS{"apps/x/r.yaml": {Data: []byte("- id: a\n  nope: 1\n")}})
	if err == nil || !strings.Contains(err.Error(), "apps/x/r.yaml:2:") {
		t.Errorf("rule file: %v", err)
	}
}

// The examples in rules/README.md ("Expression builtins") must evaluate.
func TestDocumentedBuiltins(t *testing.T) {
	f := &facts.Facts{
		CPU:    facts.CPU{Vulnerabilities: map[string]string{"meltdown": "Not affected", "mds": "Mitigation: Clear CPU buffers"}},
		Block:  []facts.BlockDev{{Name: "nvme0n1"}, {Name: "sda", Rotational: true}},
		Sysctl: map[string]string{"vm.swappiness": "10", "vm.dirty_ratio": "20"},
		Mounts: []facts.Mount{{MountPoint: "/", FSType: "ext4"}, {MountPoint: "/srv", FSType: "nfs4"}},
	}
	e := NewEnv(EnvInput{Facts: f}, nil, nil)
	cases := []struct {
		src  string
		want any
	}{
		{`all(values(facts.CPU.Vulnerabilities), {# == "Not affected" or # startsWith "Mitigation"})`, true},
		{`any(values(facts.CPU.Vulnerabilities), {# startsWith "Vulnerable"})`, false},
		{`none(facts.Block, {.Rotational})`, false},
		{`one(facts.Block, {.Rotational})`, true},
		{`count(facts.Block, {.Rotational})`, 1},
		{`len(filter(facts.Mounts, {.FSType startsWith "nfs"}))`, 1},
		{`map(facts.Block, {.Name})[0]`, "nvme0n1"},
		{`len(keys(facts.Sysctl))`, 2},
		{`"vm.swappiness" in keys(facts.Sysctl)`, true},
		{`sum(map(facts.Block, {len(.Name)}))`, 10},
		{`let n = len(facts.Block); n * 2`, 4},
	}
	for _, c := range cases {
		got, err := Eval(c.src, e)
		if err != nil || got != c.want {
			t.Errorf("%s = %v (%v), want %v", c.src, got, err, c.want)
		}
	}
}

func TestLimitNPROCIsValid(t *testing.T) {
	if !in(LimitParams, "LimitNPROC") {
		t.Errorf("LimitParams = %v", LimitParams)
	}
}

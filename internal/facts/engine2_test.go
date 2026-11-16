package facts_test

import (
	"bytes"
	"compress/gzip"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func TestEngineFacts2Mounts(t *testing.T) {
	f := collect(t, "engine-facts-2")
	if len(f.Mounts) != 6 {
		t.Fatalf("mounts = %+v", f.Mounts)
	}
	root := f.Mounts[0]
	if root.Device != "/dev/nvme0n1p2" || root.MountPoint != "/" || root.FSType != "ext4" ||
		!reflect.DeepEqual(root.Options, []string{"rw", "relatime", "errors=remount-ro"}) {
		t.Errorf("root mount = %+v", root)
	}
	cases := []struct {
		path, mountPoint, fs, disk string
	}{
		{"/", "/", "ext4", "nvme0n1"},
		{"/home/x", "/", "ext4", "nvme0n1"},
		{"/var/lib/postgresql", "/var/lib/postgresql", "xfs", "nvme0n1"}, // dm-0 -> nvme0n1p3 -> nvme0n1
		{"/var/lib/postgresql/16/main", "/var/lib/postgresql", "xfs", "nvme0n1"},
		{"/var/lib/postgresqlx", "/", "ext4", "nvme0n1"}, // component-wise prefix
		{"/srv/media/a", "/srv/media", "nfs4", ""},
		{"/dev/shm", "/dev/shm", "tmpfs", ""},
		{"/mnt/with space/data", "/mnt/with space", "ext4", "sdb"}, // \040 decoded, sdb1 -> sdb
	}
	for _, c := range cases {
		m := f.MountOf(c.path)
		if m.MountPoint != c.mountPoint || m.FSType != c.fs || m.Disk != c.disk {
			t.Errorf("MountOf(%q) = %+v, want %s %s disk %q", c.path, m, c.mountPoint, c.fs, c.disk)
		}
		if got := f.BlockDeviceOf(c.path); got != c.disk {
			t.Errorf("BlockDeviceOf(%q) = %q, want %q", c.path, got, c.disk)
		}
	}
	if !f.MountOf("/var/lib/postgresql").HasOpt("noatime") || f.MountOf("/").HasOpt("noatime") {
		t.Error("HasOpt noatime wrong")
	}
	if !f.MountOf("/dev/shm").HasOpt("size") || !f.MountOf("/dev/shm").HasOpt("size=1048576k") || f.MountOf("/dev/shm").HasOpt("size=1k") {
		t.Error("HasOpt key=value wrong")
	}
}

func TestMountsFallbackAndMissing(t *testing.T) {
	dir := t.TempDir()
	f, err := facts.Collect(facts.RootAt(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Mounts) != 0 || f.MountOf("/x").MountPoint != "" {
		t.Errorf("missing table must give no mounts: %+v", f.Mounts)
	}
	write(t, dir+"/proc/mounts", "/dev/sda1 / ext4 rw 0 0\n")
	if f, _ = facts.Collect(facts.RootAt(dir)); len(f.Mounts) != 1 || f.Mounts[0].FSType != "ext4" {
		t.Errorf("fallback to proc/mounts: %+v", f.Mounts)
	}
	write(t, dir+"/proc/self/mounts", "/dev/sda1 / xfs rw 0 0\n")
	if f, _ = facts.Collect(facts.RootAt(dir)); f.Mounts[0].FSType != "xfs" {
		t.Errorf("self/mounts must win: %+v", f.Mounts)
	}
}

func TestKernelConfig(t *testing.T) {
	f := collect(t, "engine-facts-2")
	c := f.Kernel.Config
	if c["CONFIG_BPF_JIT"] != "y" || c["CONFIG_BPF_JIT_ALWAYS_ON"] != "n" || c["CONFIG_HZ"] != "250" ||
		c["CONFIG_DEFAULT_TCP_CONG"] != "cubic" || c["CONFIG_TCP_CONG_BBR"] != "m" {
		t.Errorf("config = %v", c)
	}
	if collect(t, "minimal").Kernel.Config != nil {
		t.Error("no config file must leave Config nil")
	}

	// /proc/config.gz wins over /boot/config-<release>.
	dir := t.TempDir()
	write(t, dir+"/proc/sys/kernel/osrelease", "6.8.0\n")
	write(t, dir+"/boot/config-6.8.0", "CONFIG_BPF_JIT=n\n")
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("CONFIG_BPF_JIT=y\n# CONFIG_X is not set\n"))
	_ = zw.Close()
	if err := os.WriteFile(filepath.Join(dir, "proc", "config.gz"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := facts.Collect(facts.RootAt(dir))
	if err != nil {
		t.Fatal(err)
	}
	if g.Kernel.Config["CONFIG_BPF_JIT"] != "y" || g.Kernel.Config["CONFIG_X"] != "n" {
		t.Errorf("config.gz = %v", g.Kernel.Config)
	}
	if err := os.Remove(filepath.Join(dir, "proc", "config.gz")); err != nil {
		t.Fatal(err)
	}
	if g, _ = facts.Collect(facts.RootAt(dir)); g.Kernel.Config["CONFIG_BPF_JIT"] != "n" {
		t.Errorf("boot config fallback = %v", g.Kernel.Config)
	}
}

func TestProcOOMAndLimits(t *testing.T) {
	f := collect(t, "engine-facts-2")
	var main, child *facts.Proc
	for i := range f.Procs {
		switch f.Procs[i].PID {
		case 3100:
			main = &f.Procs[i]
		case 3101:
			child = &f.Procs[i]
		}
	}
	if main == nil || child == nil {
		t.Fatalf("procs = %+v", f.Procs)
	}
	if main.OOMScoreAdj != -900 || child.OOMScoreAdj != 0 {
		t.Errorf("oom_score_adj = %d, %d", main.OOMScoreAdj, child.OOMScoreAdj)
	}
	if main.NProcSoft != 4096 || main.NProcHard != 8192 || main.CoreSoft != math.MaxUint64 || child.CoreSoft != 0 {
		t.Errorf("limits = %+v / %+v", main, child)
	}
}

func TestDiskOfFallbacks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir+"/sys/block/sda/queue/rotational", "1\n")
	r := facts.RootAt(dir)
	if got := facts.DiskOf(r, "/dev/sda"); got != "sda" {
		t.Errorf("whole disk without class link = %q", got)
	}
	if got := facts.DiskOf(r, "/dev/mapper/nope"); got != "" {
		t.Errorf("unknown mapper = %q", got)
	}
	if got := facts.DiskOf(r, ""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func putFile(t *testing.T, dir, rel, s string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Q06/Q07/Q12/Q13/Q14: the extended privacy cases.
func TestCapturePrivacyExtended(t *testing.T) {
	dir := t.TempDir()
	put := func(rel, s string) { putFile(t, dir, rel, s) }
	put("proc/sys/kernel/osrelease", "6.1.0\n")
	put("proc/sys/kernel/hostname", "web01\n")
	put("proc/cmdline", "ro rd.luks.key=/keys/disk:s3cr3t BOOTIF=01-aa-bb-cc-dd-ee-ff ip=192.168.1.5::192.168.1.1:255.255.255.0:myhost:eth0:off iommu.passthrough=1 vconsole.keymap=us\n")
	put("proc/1/comm", "init\n")
	put("proc/1/cmdline", "/sbin/init\x00--system\x00")
	put("proc/1/cgroup", "0::/system.slice/docker-0123456789abcdef0123.scope\n0::/user.slice/user-1000.slice\n/home/carol/x\n")
	put("proc/1/fd/0", "")
	put("proc/2/comm", "sshd-session\n")
	put("proc/2/cmdline", "sshd-session: alice [priv]\x00")
	put("proc/3/comm", "postgres\n")
	put("proc/3/cmdline", "postgres: bob appdb 10.0.0.9(5432) idle\x00")
	put("proc/4/comm", "mysql\n")
	put("proc/4/cmdline", "mysql\x00-phunter2\x00")
	put("proc/5/comm", "tool\n")
	put("proc/5/cmdline", "tool\x00ghp_abcdefghijklmnopqrstuvwxyz0123456789\x00")
	put("proc/6/comm", "tool\n")
	put("proc/6/cmdline", "tool\x00Zk3Jd9QpL2xV8mNc4RtYw7Bh\x00")
	put("proc/7/comm", "app\n")
	put("proc/7/cmdline", "/home/dave/bin/app\x00--password=hunter3\x00")
	put("proc/8/comm", "app\n")
	put("proc/8/cmdline", "app\x00--addr=fe80::1ff:fe23:4567:890a%eth0\x00")
	put("proc/9/comm", "app\n")
	put("proc/9/cmdline", "app\x00--mac=aa:bb:cc:dd:ee:ff\x00")
	put("proc/mounts", "/dev/sda1 /home/erin/data ext4 rw 0 0\noverlay /var/lib/docker/overlay2/0123456789abcdef0123456789abcdef/merged overlay rw 0 0\n/dev/sdb1 /Users/frank xfs rw 0 0\n/dev/sdc /media/gina/usb vfat rw 0 0\n")
	put("sys/class/dmi/id/chassis_asset_tag", "ASSET-ACME-12345\n")
	put("sys/class/dmi/id/sys_vendor", "ACME\n")

	snap := captureRoot(facts.RootAt(dir))
	var all strings.Builder
	for k, v := range snap.Files {
		all.WriteString(k + "=" + v + "\n")
	}
	for _, leak := range []string{"alice", "bob", "appdb", "hunter2", "hunter3", "ghp_abcdef", "Zk3Jd9", "s3cr3t", "aa-bb-cc", "aa:bb:cc",
		"fe80", "192.168.1.5", "myhost", "carol", "dave", "erin", "frank", "gina", "ASSET-ACME", "0123456789abcdef", "10.0.0.9"} {
		if strings.Contains(all.String(), leak) {
			t.Errorf("leaked %q in:\n%s", leak, all.String())
		}
	}
	if got := snap.Files["proc/2/cmdline"]; got != "sshd-session: [redacted]\x00" {
		t.Errorf("sshd-session title = %q", got)
	}
	if got := snap.Files["proc/3/cmdline"]; got != "postgres: [redacted]\x00" {
		t.Errorf("postgres title = %q", got)
	}
	if got := snap.Files["proc/4/cmdline"]; got != "mysql\x00-p[redacted]\x00" {
		t.Errorf("-p = %q", got)
	}
	if got := snap.Files["proc/5/cmdline"]; got != "tool\x00[redacted]\x00" {
		t.Errorf("token = %q", got)
	}
	if got := snap.Files["proc/7/cmdline"]; got != "/home/[user]/bin/app\x00--password=[redacted]\x00" {
		t.Errorf("home path = %q", got)
	}
	if got := snap.Files["proc/cmdline"]; !strings.Contains(got, "iommu.passthrough=1") || !strings.Contains(got, "vconsole.keymap=us") {
		t.Errorf("benign parameters were redacted: %q", got)
	}
	if got := snap.Files["proc/mounts"]; !strings.Contains(got, "/home/[user]/data") || !strings.Contains(got, "/Users/[user]") || !strings.Contains(got, "/media/[user]/usb") || !strings.Contains(got, "[id]") {
		t.Errorf("mounts = %q", got)
	}
	if got := snap.Files["proc/1/cgroup"]; !strings.Contains(got, "[id]") {
		t.Errorf("proc/1/cgroup not scrubbed: %q", got)
	}
	if got := snap.Files["sys/class/dmi/id/chassis_asset_tag"]; got != "" {
		t.Errorf("unknown asset tag kept: %q", got)
	}
}

func TestCaptureKeepsCloudAssetTag(t *testing.T) {
	dir := t.TempDir()
	putFile(t, dir, "proc/sys/kernel/osrelease", "6.1.0\n")
	putFile(t, dir, "sys/class/dmi/id/chassis_asset_tag", "7783-7084-3265-9085-8269-3286-77\n")
	snap := captureRoot(facts.RootAt(dir))
	if got := snap.Files["sys/class/dmi/id/chassis_asset_tag"]; !strings.Contains(got, "7783-7084") {
		t.Errorf("azure tag dropped: %q", got)
	}
}

// Q14: the host name comes from the captured root only, on word boundaries.
func TestCaptureHostnameWordBoundary(t *testing.T) {
	dir := t.TempDir()
	putFile(t, dir, "proc/sys/kernel/osrelease", "6.1.0\n")
	putFile(t, dir, "proc/sys/kernel/hostname", "web01\n")
	putFile(t, dir, "proc/1/comm", "x\n")
	putFile(t, dir, "proc/1/cmdline", "/usr/bin/web01\x00--note=web01x-notweb01\x00")
	host, _ := os.Hostname()
	c := &capturer{r: facts.RootAt(dir)}
	c.initHosts()
	if got := c.scrub("host web01 and web01x and xweb01 and web01."); got != "host redacted-host and web01x and xweb01 and redacted-host." {
		t.Errorf("scrub = %q", got)
	}
	for _, h := range c.hosts {
		if host != "" && strings.EqualFold(h, host) && host != "web01" {
			t.Errorf("os.Hostname %q leaked into the matcher", host)
		}
	}
}

// Q08: -o never overwrites or follows symlinks; --force replaces atomically.
func TestCaptureOutputExclusive(t *testing.T) {
	dir := t.TempDir()
	putFile(t, dir, "proc/sys/kernel/osrelease", "6.1.0\n")
	out := filepath.Join(t.TempDir(), "cap.tar.gz")
	run := func(args ...string) error {
		root := NewRootCmd()
		var b bytes.Buffer
		root.SetOut(&b)
		root.SetErr(&b)
		root.SetArgs(append([]string{"capture", "--root", dir}, args...))
		return root.Execute()
	}
	if err := run("-o", out); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(out); st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", st.Mode().Perm())
	}
	if err := run("-o", out); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("overwrite not refused: %v", err)
	}
	if err := run("-o", out, "--force"); err != nil {
		t.Fatalf("--force: %v", err)
	}
	// planted symlink
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := run("-o", link); err == nil {
		t.Fatal("symlink output accepted")
	}
	if err := run("-o", link, "--force"); err != nil {
		t.Fatalf("--force over a symlink: %v", err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("symlink target overwritten: %q", b)
	}
}

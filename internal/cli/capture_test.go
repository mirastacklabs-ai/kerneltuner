package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixturetest"
)

func runCapture(t *testing.T, args ...string) []byte {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs(append([]string{"capture"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("capture %v: %v", args, err)
	}
	return out.Bytes()
}

// A fixture captured and loaded back must collect to identical facts.
func TestCaptureFixtureRoundTrip(t *testing.T) {
	for _, name := range []string{"web-16c-64g-nginx", "gpu-8xh100", "container-k8s-redis", "legacy-4.9-c10k", "bpftune-tuned-host", "gpu-8xmi300x"} {
		t.Run(name, func(t *testing.T) {
			src, _, _ := fixturetest.Load(t, name)
			want, err := facts.Collect(src)
			if err != nil {
				t.Fatal(err)
			}
			srcDir := filepath.Dir(src.Proc)
			yml := runCapture(t, "--root", srcDir, "--fixture")

			base := t.TempDir()
			if err := os.MkdirAll(filepath.Join(base, "rt"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(base, "rt", "fixture.yaml"), yml, 0o644); err != nil {
				t.Fatal(err)
			}
			spec, err := fixture.ParseIn(base, "rt")
			if err != nil {
				t.Fatalf("captured fixture does not parse: %v", err)
			}
			if spec.Synthetic {
				t.Error("captured fixture must be synthetic: false")
			}
			dst, _, _, err := fixture.MaterializeSpec(t.TempDir(), spec)
			if err != nil {
				t.Fatal(err)
			}
			got, err := facts.Collect(dst)
			if err != nil {
				t.Fatal(err)
			}
			want.Warnings, got.Warnings = nil, nil
			// Documented lossy steps: fd placeholders are capped, command lines keep
			// argv[0..1] (redacted), and the kernel command line is redacted.
			for i := range want.Procs {
				p := &want.Procs[i]
				p.OpenFDs = min(p.OpenFDs, maxFDEntries)
				p.Cmdline = splitNULArgs(keepArgv(strings.Join(p.Cmdline, "\x00") + "\x00"))
			}
			if want.Kernel.Release != got.Kernel.Release {
				t.Errorf("release %q != %q", want.Kernel.Release, got.Kernel.Release)
			}
			want.Kernel.Cmdline, want.Kernel.CmdlineArgs = got.Kernel.Cmdline, got.Kernel.CmdlineArgs
			if !reflect.DeepEqual(want, got) {
				t.Errorf("facts differ after round trip\nwant %+v\ngot  %+v", want.Kernel, got.Kernel)
				diffFacts(t, want, got)
			}
		})
	}
}

func splitNULArgs(s string) []string {
	s = strings.TrimRight(s, "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

func diffFacts(t *testing.T, a, b *facts.Facts) {
	t.Helper()
	checks := map[string][2]any{
		"Kernel": {a.Kernel, b.Kernel}, "Host": {a.Host, b.Host}, "CPU": {a.CPU, b.CPU}, "NUMA": {a.NUMA, b.NUMA},
		"Memory": {a.Memory, b.Memory}, "NICs": {a.NICs, b.NICs}, "Block": {a.Block, b.Block}, "Accel": {a.Accel, b.Accel},
		"RDMA": {a.RDMA, b.RDMA}, "IOMMU": {a.IOMMU, b.IOMMU}, "Sysctl": {a.Sysctl, b.Sysctl}, "Limits": {a.Limits, b.Limits},
		"Modules": {a.Modules, b.Modules}, "Procs": {a.Procs, b.Procs}, "Tuned": {a.Tuned, b.Tuned},
	}
	for k, v := range checks {
		if !reflect.DeepEqual(v[0], v[1]) {
			t.Errorf("%s differs", k)
		}
	}
}

func TestCapturePrivacy(t *testing.T) {
	dir := t.TempDir()
	put := func(rel, s string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("proc/sys/kernel/osrelease", "6.1.0\n")
	put("proc/sys/kernel/hostname", "secret-box-07\n")
	put("proc/sys/kernel/pid_max", "4194304\n")
	put("proc/sys/net/ipv4/tcp_fastopen_key", "deadbeef-deadbeef-deadbeef-deadbeef\n")
	put("proc/cmdline", "root=UUID=3b1f4a6e-1c2d-4e5f-8a9b-0c1d2e3f4a5b ro ip=10.1.2.3 iommu=pt\n")
	put("proc/1/comm", "init\n")
	put("proc/1/cmdline", "/sbin/init\x00--system\x00--secret-arg\x00more\x00")
	put("proc/1/environ", "AWS_SECRET_ACCESS_KEY=abc\x00container=podman\x00HOME=/root\x00")
	put("proc/1/cgroup", "0::/init.scope\n")
	put("proc/1/limits", "Max open files            1024                 4096                 files\n")
	put("proc/1/fd/0", "")
	put("proc/1/fd/1", "")
	put("proc/2/comm", "sshd\n")
	put("proc/2/cmdline", "sshd: alice@pts/0\x00")
	put("proc/3/comm", "app\n")
	put("proc/3/cmdline", "/usr/bin/app\x00--db-password=hunter2\x00--host\x00db1.prod.example.com\x00")
	put("proc/4/comm", "app2\n")
	put("proc/4/cmdline", "/usr/bin/app2\x00--listen=10.9.8.7:80\x00x\x00")
	put("proc/net/tcp", "  sl  local_address rem_address   st\n   0: 0A0A0A0A:0050 00000000:0000 0A 0 0\n   1: 0A0A0A0A:D431 0B0B0B0B:01BB 01 0 0\n")
	put("sys/class/dmi/id/sys_vendor", "ACME\n")
	put("sys/class/dmi/id/product_serial", "SERIAL123\n")
	put("sys/class/dmi/id/product_uuid", "11111111-2222-3333-4444-555555555555\n")
	put("sys/class/net/eth0/address", "aa:bb:cc:dd:ee:ff\n")
	put("sys/class/net/eth0/mtu", "1500\n")
	put("sys/not/on/allowlist", "x\n")
	put("etc/shadow", "root:$6$hash\n")
	put("etc/hostname", "secret-box-07\n")

	snap := captureRoot(facts.RootAt(dir))

	for k := range snap.Files {
		for _, bad := range []string{"shadow", "hostname", "product_serial", "product_uuid", "address", "allowlist", "tcp_fastopen_key"} {
			if strings.HasSuffix(k, bad) {
				t.Errorf("non-allowlisted file captured: %s", k)
			}
		}
	}
	var all strings.Builder
	for k, v := range snap.Files {
		all.WriteString(k + "=" + v + "\n")
	}
	for _, leak := range []string{"hunter2", "alice", "AWS_SECRET", "HOME=", "secret-box-07", "10.1.2.3", "10.9.8.7", "example.com", "3b1f4a6e", "0B0B0B0B", "0A0A0A0A"} {
		if strings.Contains(all.String(), leak) {
			t.Errorf("leaked %q", leak)
		}
	}
	if got := snap.Files["proc/1/environ"]; got != "container=podman\x00" {
		t.Errorf("environ = %q", got)
	}
	if got := snap.Files["proc/1/cmdline"]; got != "/sbin/init\x00--system\x00" {
		t.Errorf("cmdline = %q", got)
	}
	if got := snap.Files["proc/3/cmdline"]; got != "/usr/bin/app\x00--db-password=[redacted]\x00" {
		t.Errorf("secret cmdline = %q", got)
	}
	if got := snap.Files["proc/2/cmdline"]; got != "sshd: [redacted]\x00" {
		t.Errorf("sshd cmdline = %q", got)
	}
	if got := snap.Files["proc/cmdline"]; !strings.Contains(got, "iommu=pt") || !strings.Contains(got, "root=UUID=[uuid]") || !strings.Contains(got, "ip=[ip]") {
		t.Errorf("kernel cmdline = %q", got)
	}
	if got := snap.Files["proc/net/tcp"]; strings.Count(got, "\n") != 2 || !strings.Contains(got, "00000000:0050") {
		t.Errorf("tcp = %q", got)
	}
	if _, ok := snap.Files["proc/1/fd/1"]; !ok || snap.Files["proc/1/fd/1"] != "" {
		t.Error("fd placeholders missing")
	}
	// The collector sees the recorded fd count and the container signal.
	if snap.Files["sys/class/dmi/id/sys_vendor"] != "ACME\n" {
		t.Error("dmi sys_vendor lost")
	}
}

func TestCaptureArchive(t *testing.T) {
	src, _, _ := fixturetest.Load(t, "gpu-8xh100")
	out := filepath.Join(t.TempDir(), "cap.tar.gz")
	runCapture(t, "--root", filepath.Dir(src.Proc), "-o", out)

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	names := map[string]string{}
	links := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeSymlink {
			links[h.Name] = h.Linkname
			continue
		}
		b, _ := io.ReadAll(tr)
		names[h.Name] = string(b)
	}
	if names["proc/sys/kernel/osrelease"] != "6.8.0-45-generic\n" {
		t.Errorf("osrelease = %q", names["proc/sys/kernel/osrelease"])
	}
	if links["sys/bus/pci/devices/0000:18:00.0/driver"] != "../../../bus/pci/drivers/nvidia" {
		t.Errorf("driver link = %q", links["sys/bus/pci/devices/0000:18:00.0/driver"])
	}
	if !strings.HasPrefix(names["sys/bus/pci/devices/0000:ab:00.0/current_link_width"], "8") {
		t.Error("narrow GPU link width missing")
	}
	for n := range names {
		if strings.Contains(n, "environ") {
			t.Errorf("environ captured: %s", n)
		}
	}
}

func TestCaptureNeedsOutput(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"capture"})
	if err := root.Execute(); err == nil {
		t.Fatal("capture without -o must fail")
	}
}

func TestRedactArg(t *testing.T) {
	cases := map[string]string{
		"--port=8080":                        "--port=8080",
		"vllm.entrypoints.openai.api_server": "vllm.entrypoints.openai.api_server",
		"--token=abc":                        "--token=[redacted]",
		"db.internal":                        "[host]",
		"0.0.0.0:80":                         "0.0.0.0:80",
		"192.168.1.5":                        "[ip]",
		"user@host":                          "[redacted]",
	}
	for in, want := range cases {
		if got := redactArg(in); got != want {
			t.Errorf("redactArg(%q) = %q, want %q", in, got, want)
		}
	}
}

package render

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

var update = flag.Bool("update", false, "rewrite golden files")

const src = "https://docs.kernel.org/networking/ip-sysctl.html"

func testMeta() Meta {
	return Meta{
		Version: "v0.0.0-test",
		Date:    time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC),
		Facts: &facts.Facts{
			Kernel:   facts.Kernel{Release: "6.8.0-31-generic"},
			CPU:      facts.CPU{Model: "EPYC 9654", Sockets: 2, Cores: 192, Threads: 384},
			Memory:   facts.Memory{TotalBytes: 1536 << 30},
			Accel:    []facts.Accelerator{{Vendor: "nvidia"}, {Vendor: "nvidia"}, {Vendor: "amd"}},
			Tuned:    facts.Tuned{Active: true, Profile: "network-throughput"},
			Warnings: []string{"could not read /proc/net/netstat"},
		},
	}
}

func testRecs() []advisor.Recommendation {
	ev := []advisor.EvidenceHit{{Metric: "TcpExt.ListenOverflows", Value: 3.2, Threshold: 0}}
	return []advisor.Recommendation{
		{RuleID: "net.core.somaxconn", Kind: "sysctl", Domain: "network", Param: "net.core.somaxconn", Title: "Listen backlog cap",
			Current: "128", Proposed: "4096", Status: advisor.StatusRecommend, Risk: "low", Score: 5, Evidence: ev,
			Rationale: "Small backlog drops connections. Raised to 4096 in kernel 5.4.", Sources: []string{src}},
		{RuleID: "net.ipv4.tcp_rmem", Kind: "sysctl", Domain: "network", Param: "net.ipv4.tcp_rmem", Title: "TCP receive buffer",
			Current: "4096 131072 6291456", Proposed: "4096 131072 16777216", Status: advisor.StatusRecommend, Risk: "medium", Score: 0.7,
			Rationale: "Long fat pipes need larger buffers.", Sources: []string{src}},
		{RuleID: "net.ipv4.tcp_congestion_control", Kind: "sysctl", Domain: "network", Param: "net.ipv4.tcp_congestion_control",
			Current: "bbr", Proposed: "bbr", Status: advisor.StatusOK, Risk: "low", Rationale: "BBR.", Sources: []string{src}},
		{RuleID: "vm.swappiness", Kind: "sysctl", Domain: "memory", Param: "vm.swappiness", Title: "Swappiness", Current: "60", Proposed: "10",
			Status: advisor.StatusManaged, ManagedBy: "tuned:network-throughput", Risk: "low", Rationale: "Avoid swap.",
			Advice: "tuned profile sets vm.swappiness=10", Sources: []string{src}},
		{RuleID: "limits.nofile", Kind: "limit", Domain: "limits", Param: "LimitNOFILE", Target: "nginx.service", Title: "Open files for nginx",
			Current: "1024", Proposed: "65536", Status: advisor.StatusRecommend, Risk: "low", Score: 1, Rationale: "Default is too low.", Sources: []string{src}},
		{RuleID: "limits.memlock", Kind: "limit", Domain: "limits", Param: "LimitMEMLOCK", Target: "vllm", Title: "Locked memory for vllm",
			Current: "65536", Proposed: "infinity", Status: advisor.StatusRecommend, Risk: "low", Score: 1, Rationale: "Pinned buffers.", Sources: []string{src}},
		{RuleID: "storage.scheduler", Kind: "sysfs", Domain: "storage", Param: "/sys/block/{item}/queue/scheduler", Target: "nvme0n1", Title: "NVMe scheduler",
			Current: "mq-deadline", Proposed: "none", Status: advisor.StatusRecommend, Risk: "low", Score: 1, Rationale: "NVMe has its own queues.", Sources: []string{src}},
		{RuleID: "cmdline.iommu", Kind: "cmdline", Domain: "ai", Param: "iommu", Title: "IOMMU passthrough", Current: "", Proposed: "pt",
			Status: advisor.StatusRecommend, Risk: "medium", Reboot: true, Score: 0.7, Rationale: "Lower DMA overhead.", Sources: []string{src}},
		{RuleID: "cmdline.nosmt", Kind: "cmdline", Domain: "ai", Param: "nohz_full", Title: "Tickless cores", Proposed: "",
			Status: advisor.StatusRecommend, Risk: "high", Reboot: true, Score: 0.4, Rationale: "Fewer interrupts.", Sources: []string{src}},
		{RuleID: "nic.ring", Kind: "nic", Domain: "network", Param: "rx_ring", Target: "eth0", Title: "RX ring", Current: "256", Proposed: "4096",
			Status: advisor.StatusRecommend, Risk: "low", Score: 1, Rationale: "Absorb bursts.", Sources: []string{src}},
		{RuleID: "nic.txring", Kind: "nic", Domain: "network", Param: "tx_ring", Target: "eth0", Title: "TX ring", Current: "256", Proposed: "2048",
			Status: advisor.StatusRecommend, Risk: "low", Score: 1, Rationale: "Absorb bursts.", Sources: []string{src}},
		{RuleID: "nic.channels", Kind: "nic", Domain: "network", Param: "combined", Target: "eth0", Title: "Channels", Current: "4", Proposed: "32",
			Status: advisor.StatusRecommend, Risk: "low", Score: 1, Rationale: "One per core.", Sources: []string{src}},
		{RuleID: "rt.vllm", Kind: "runtime", Domain: "ai", Target: "vllm", Title: "vLLM batch size", Status: advisor.StatusRecommend, Risk: "low",
			Score: 1, Advice: "Set --max-num-seqs to 256.", Rationale: "Throughput.", Verify: "Watch tokens/s", Sources: []string{src}},
		{RuleID: "anti.thp", Kind: "antipattern", Domain: "memory", Title: "THP always on database host", Status: advisor.StatusFiring, Risk: "medium",
			Score: 1.05, Rationale: "THP stalls latency-sensitive databases. Use madvise.", Sources: []string{src}},
		{RuleID: "broken", Kind: "sysctl", Domain: "cpu", Title: "Broken rule", Status: advisor.StatusError, Risk: "low",
			Advice: "error: recommend: boom"},
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden file.\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestTextGolden(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, testRecs(), testMeta(), false); err != nil {
		t.Fatal(err)
	}
	golden(t, "scan.txt.golden", b.String())
	b.Reset()
	if err := Text(&b, testRecs(), testMeta(), true); err != nil {
		t.Fatal(err)
	}
	golden(t, "scan-all.txt.golden", b.String())
	if !strings.Contains(b.String(), "Warnings: 1") {
		t.Error("header lacks warnings count")
	}
}

func TestTextEmpty(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, nil, Meta{}, false); err != nil || !strings.Contains(b.String(), "Nothing to recommend") {
		t.Errorf("%v %q", err, b.String())
	}
}

func TestJSONGolden(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, testRecs(), testMeta(), false); err != nil {
		t.Fatal(err)
	}
	golden(t, "scan.json.golden", b.String())
	var rep JSONReport
	if err := json.Unmarshal(b.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Recommendations {
		if r.Status == "ok" {
			t.Errorf("ok result leaked into default JSON: %+v", r)
		}
	}
	b.Reset()
	if err := JSON(&b, nil, Meta{}, true); err != nil || !strings.Contains(b.String(), `"recommendations": []`) {
		t.Errorf("empty JSON must have [] not null: %q", b.String())
	}
}

func TestGenerateGolden(t *testing.T) {
	files, err := Generate("all", testRecs(), testMeta())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	want := []string{
		"90-kerneltuner.conf", "grub-cmdline.txt", "limits.d/90-kerneltuner.conf", "nic.sh", "runtime.md",
		"systemd/nginx.service.d/90-kerneltuner.conf", "tuned.conf",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("files = %v, want %v", names, want)
	}
	for n, body := range files {
		golden(t, "generate/"+n+".golden", body)
		if !strings.Contains(body, "Generated by kerneltuner v0.0.0-test on 2026-11-16") || !strings.Contains(body, "Review before applying") {
			t.Errorf("%s lacks the header", n)
		}
	}
	if strings.Contains(files["90-kerneltuner.conf"], "vm.swappiness") || strings.Contains(files["90-kerneltuner.conf"], "cmdline") {
		t.Error("sysctl file contains non-recommend or non-sysctl entries")
	}
	if !strings.Contains(files["tuned.conf"], "include=network-throughput") {
		t.Error("tuned include should be the active profile")
	}
}

func TestGenerateSubsetsAndErrors(t *testing.T) {
	files, err := Generate("systemd", testRecs(), testMeta())
	if err != nil || len(files) != 1 {
		t.Errorf("systemd: %v %v", files, err)
	}
	files, _ = Generate("limits", testRecs(), testMeta())
	if len(files) != 2 {
		t.Errorf("limits: %v", files)
	}
	if _, err := Generate("bogus", nil, Meta{}); err == nil {
		t.Error("unknown format accepted")
	}
	files, err = Generate("all", nil, Meta{})
	if err != nil || len(files) != 0 {
		t.Errorf("nothing to generate: %v %v", files, err)
	}
	m := testMeta()
	m.Facts.Tuned = facts.Tuned{}
	if !strings.Contains(Tuned(testRecs(), m), "include=throughput-performance") {
		t.Error("default tuned include")
	}
}

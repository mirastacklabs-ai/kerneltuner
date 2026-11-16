package accel_test

import (
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts/accel"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
)

func TestClassify(t *testing.T) {
	cases := []struct{ vendor, class, want string }{
		{"0x10de", "0x030200", "nvidia"},
		{"0x10de", "0x030000", "nvidia"},
		{"0x10de", "0x068000", ""}, // NVSwitch bridge
		{"0x10de", "0x040300", ""}, // HDMI audio
		{"0x1002", "0x038000", "amd"},
		{"0x1002", "0x120000", "amd"},
		{"0x1002", "0x060400", ""},
		{"0x8086", "0x038000", "intel"},
		{"0x8086", "0x120000", "intel"},
		{"0x8086", "0x020000", ""}, // Ethernet
		{"0x8086", "0x030000", ""}, // iGPU
		{"0x1da3", "0x120000", "habana"},
		{"0x15b3", "0x020000", ""},
	}
	for _, c := range cases {
		if got := accel.Classify(c.vendor, c.class); got != c.want {
			t.Errorf("Classify(%s,%s) = %q, want %q", c.vendor, c.class, got, c.want)
		}
	}
}

func TestScan(t *testing.T) {
	root, _, _ := fixture.Load(t, "gpu-8xh100")
	devs, err := accel.Scan(root.Sys)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 8 {
		t.Fatalf("devices = %d", len(devs))
	}
	d := devs[5] // 0000:ab:00.0
	if d.PCIAddr != "0000:ab:00.0" || d.Vendor != "nvidia" || d.DeviceID != "0x2330" || d.Driver != "nvidia" ||
		d.NUMANode != 1 || d.LinkWidth != 8 || d.LinkWidthMax != 16 || d.LinkSpeed != "32.0 GT/s PCIe" || d.IOMMUGroup != "45" {
		t.Errorf("device = %+v", d)
	}
}

func TestScanMissingLinkInfo(t *testing.T) {
	root, _, _ := fixture.Load(t, "gaudi3-8x")
	devs, err := accel.Scan(root.Sys)
	if err != nil || len(devs) != 8 || devs[0].Vendor != "habana" {
		t.Fatalf("devs=%v err=%v", devs, err)
	}
	empty := t.TempDir()
	if _, err := accel.Scan(empty); err == nil {
		t.Error("missing PCI bus must return an error")
	}
}

func TestScanRDMA(t *testing.T) {
	root, _, _ := fixture.Load(t, "gpu-8xmi300x")
	devs, err := accel.ScanRDMA(root.Sys)
	if err != nil || len(devs) != 8 {
		t.Fatalf("devs=%d err=%v", len(devs), err)
	}
	if p := devs[0].Ports[0]; p.LinkLayer != "Ethernet" || p.NetDev != "ens10f0np0" || p.State != "4: ACTIVE" {
		t.Errorf("port = %+v", p)
	}
	none, err := accel.ScanRDMA(t.TempDir())
	if err != nil || none != nil {
		t.Errorf("no RDMA: %v %v", none, err)
	}
}

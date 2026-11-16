// Package accel detects accelerators (NVIDIA, AMD, Intel, Habana) and RDMA devices via sysfs PCI.
//
// It has no dependency on the facts package: the facts collector converts
// these results into its own types.
package accel

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Device is one accelerator PCI function.
type Device struct {
	Vendor                                     string // "nvidia","amd","intel","habana"
	PCIAddr, VendorID, DeviceID, Class, Driver string
	NUMANode                                   int // -1 when unknown
	LinkSpeed, LinkSpeedMax                    string
	LinkWidth, LinkWidthMax                    int
	IOMMUGroup                                 string
}

// Classify maps a PCI vendor ID ("0x10de") and class ("0x030200") to an
// accelerator vendor name, or "" when the function is not an accelerator.
func Classify(vendorID, class string) string {
	v := strings.ToLower(strings.TrimSpace(vendorID))
	c := strings.ToLower(strings.TrimSpace(class))
	switch v {
	case "0x10de":
		if strings.HasPrefix(c, "0x03") { // 0x0300 VGA, 0x0302 3D controller
			return "nvidia"
		}
	case "0x1002":
		if strings.HasPrefix(c, "0x03") || strings.HasPrefix(c, "0x12") {
			return "amd"
		}
	case "0x8086":
		if strings.HasPrefix(c, "0x0380") || strings.HasPrefix(c, "0x12") {
			return "intel"
		}
	case "0x1da3":
		return "habana"
	}
	return ""
}

// Scan walks <sys>/bus/pci/devices and returns the accelerators found,
// sorted by PCI address. A missing PCI bus yields an error.
func Scan(sys string) ([]Device, error) {
	dir := filepath.Join(sys, "bus", "pci", "devices")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Device
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		vendor := readTrim(filepath.Join(p, "vendor"))
		class := readTrim(filepath.Join(p, "class"))
		kind := Classify(vendor, class)
		if kind == "" {
			continue
		}
		d := Device{
			Vendor:       kind,
			PCIAddr:      e.Name(),
			VendorID:     strings.ToLower(vendor),
			DeviceID:     strings.ToLower(readTrim(filepath.Join(p, "device"))),
			Class:        strings.ToLower(class),
			Driver:       linkBase(filepath.Join(p, "driver")),
			NUMANode:     -1,
			LinkSpeed:    readTrim(filepath.Join(p, "current_link_speed")),
			LinkSpeedMax: readTrim(filepath.Join(p, "max_link_speed")),
			LinkWidth:    atoi(readTrim(filepath.Join(p, "current_link_width"))),
			LinkWidthMax: atoi(readTrim(filepath.Join(p, "max_link_width"))),
			IOMMUGroup:   linkBase(filepath.Join(p, "iommu_group")),
		}
		if s := readTrim(filepath.Join(p, "numa_node")); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				d.NUMANode = n
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PCIAddr < out[j].PCIAddr })
	return out, nil
}

// RDMAPort is one port of an RDMA device.
type RDMAPort struct {
	Num                            int
	State, LinkLayer, Rate, NetDev string
}

// RDMADev is an InfiniBand/RoCE device.
type RDMADev struct {
	Name  string
	Ports []RDMAPort
}

// ScanRDMA reads <sys>/class/infiniband. A missing directory returns nil, nil
// (the host simply has no RDMA devices).
func ScanRDMA(sys string) ([]RDMADev, error) {
	dir := filepath.Join(sys, "class", "infiniband")
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []RDMADev
	for _, e := range ents {
		dev := RDMADev{Name: e.Name()}
		base := filepath.Join(dir, e.Name())
		netdev := ""
		if nets, err := os.ReadDir(filepath.Join(base, "device", "net")); err == nil && len(nets) > 0 {
			netdev = nets[0].Name()
		}
		ports, _ := os.ReadDir(filepath.Join(base, "ports"))
		for _, pe := range ports {
			n, err := strconv.Atoi(pe.Name())
			if err != nil {
				continue
			}
			pp := filepath.Join(base, "ports", pe.Name())
			dev.Ports = append(dev.Ports, RDMAPort{
				Num:       n,
				State:     readTrim(filepath.Join(pp, "state")),
				LinkLayer: readTrim(filepath.Join(pp, "link_layer")),
				Rate:      readTrim(filepath.Join(pp, "rate")),
				NetDev:    netdev,
			})
		}
		sort.Slice(dev.Ports, func(i, j int) bool { return dev.Ports[i].Num < dev.Ports[j].Num })
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func linkBase(p string) string {
	t, err := os.Readlink(p)
	if err != nil {
		return ""
	}
	return filepath.Base(t)
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

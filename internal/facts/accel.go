package facts

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts/accel"
)

func collectAccel(r Root, f *Facts) {
	devs, err := accel.Scan(r.Sys)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) { // no PCI bus (fixtures, containers) is not worth a warning
			f.warnf("cannot scan PCI devices: %v", err)
		}
		return
	}
	for _, d := range devs {
		f.Accel = append(f.Accel, Accelerator{
			Vendor: d.Vendor, PCIAddr: d.PCIAddr, VendorID: d.VendorID, DeviceID: d.DeviceID,
			Class: d.Class, Driver: d.Driver, NUMANode: d.NUMANode,
			LinkSpeed: d.LinkSpeed, LinkSpeedMax: d.LinkSpeedMax,
			LinkWidth: d.LinkWidth, LinkWidthMax: d.LinkWidthMax,
			IOMMUGroup: d.IOMMUGroup, Kind: d.Kind,
		})
	}
}

func collectRDMA(r Root, f *Facts) {
	devs, err := accel.ScanRDMA(r.Sys)
	if err != nil {
		f.warnf("cannot scan RDMA devices: %v", err)
		return
	}
	for _, d := range devs {
		rd := RDMADev{Name: d.Name}
		for _, p := range d.Ports {
			rd.Ports = append(rd.Ports, RDMAPort(p))
		}
		f.RDMA = append(f.RDMA, rd)
	}
}

func collectIOMMU(r Root, f *Facts) {
	m := &f.IOMMU
	if ents, err := os.ReadDir(filepath.Join(r.Sys, "class/iommu")); err == nil && len(ents) > 0 {
		m.Enabled = true
	}
	groups, _ := os.ReadDir(filepath.Join(r.Sys, "kernel/iommu_groups"))
	m.Groups = len(groups)
	if v, ok := f.Kernel.CmdlineArgs["iommu"]; ok && v == "pt" {
		m.Passthrough = true
	}
	if _, ok := f.Kernel.CmdlineArgs["iommu.passthrough"]; ok && f.Kernel.CmdlineArgs["iommu.passthrough"] == "1" {
		m.Passthrough = true
	}
	if !m.Passthrough && len(groups) > 0 {
		identity := 0
		for _, g := range groups {
			if readOpt(filepath.Join(r.Sys, "kernel/iommu_groups", g.Name(), "type")) == "identity" {
				identity++
			}
		}
		m.Passthrough = identity == len(groups)
	}
	if m.Groups > 0 {
		m.Enabled = true
	}
}

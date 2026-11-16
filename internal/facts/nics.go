package facts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Ring holds channel and ring sizes for one interface; 0 means unknown.
// The JSON form is also the fixture-only file
// <Root.Run>/kerneltuner-fixture/ethtool/<if>.json.
type Ring struct {
	Combined    int `json:"combined"`
	CombinedMax int `json:"combined_max"`
	RxRing      int `json:"rx_ring"`
	RxRingMax   int `json:"rx_ring_max"`
	TxRing      int `json:"tx_ring"`
	TxRingMax   int `json:"tx_ring_max"`
}

// FixtureEthtoolPath is where fixtures keep ring/channel sizes for interface ifname.
func FixtureEthtoolPath(r Root, ifname string) string {
	return filepath.Join(r.Run, "kerneltuner-fixture", "ethtool", ifname+".json")
}

func collectNICs(r Root, f *Facts) {
	base := filepath.Join(r.Sys, "class/net")
	ents, err := os.ReadDir(base)
	if err != nil {
		f.warnf("cannot read %s: %v", base, err)
	}
	have := map[string]bool{}
	for _, e := range ents {
		name := e.Name()
		if name == "lo" {
			continue
		}
		f.NICs = append(f.NICs, buildNIC(r, name, filepath.Join(base, name), !exists(filepath.Join(base, name, "device"))))
		have[name] = true
	}
	// RoCE netdevs that only appear under infiniband/<dev>/device/net/<if>.
	ibNets, _ := filepath.Glob(filepath.Join(r.Sys, "class/infiniband/*/device/net/*"))
	for _, dir := range ibNets {
		name := filepath.Base(dir)
		if name == "lo" || have[name] {
			continue
		}
		have[name] = true
		f.NICs = append(f.NICs, buildNIC(r, name, dir, false))
	}
	sort.Slice(f.NICs, func(i, j int) bool { return f.NICs[i].Name < f.NICs[j].Name })
}

// buildNIC reads one interface from dir (a class/net entry, or the netdev
// directory behind an RDMA device).
func buildNIC(r Root, name, dir string, virtual bool) NIC {
	n := NIC{
		Name:     name,
		Virtual:  virtual,
		MTU:      atoi(readOpt(filepath.Join(dir, "mtu"))),
		NUMANode: atoiDef(readOpt(filepath.Join(dir, "device/numa_node")), -1),
		Driver:   linkBase(filepath.Join(dir, "device/driver")),
	}
	if sp := atoi(readOpt(filepath.Join(dir, "speed"))); sp > 0 {
		n.SpeedMbps = sp
	}
	if q, err := os.ReadDir(filepath.Join(dir, "queues")); err == nil {
		for _, qe := range q {
			switch {
			case strings.HasPrefix(qe.Name(), "rx-"):
				n.RxQueues++
			case strings.HasPrefix(qe.Name(), "tx-"):
				n.TxQueues++
			}
		}
	}
	if ring, ok := ringFor(r, name); ok {
		n.Combined, n.CombinedMax = ring.Combined, ring.CombinedMax
		n.RxRing, n.RxRingMax = ring.RxRing, ring.RxRingMax
		n.TxRing, n.TxRingMax = ring.TxRing, ring.TxRingMax
	}
	return n
}

// ringFor prefers the fixture file, then (real /sys only) the ethtool ioctl.
func ringFor(r Root, ifname string) (Ring, bool) {
	if b, err := os.ReadFile(FixtureEthtoolPath(r, ifname)); err == nil {
		var ring Ring
		if json.Unmarshal(b, &ring) == nil {
			return ring, true
		}
		return Ring{}, false
	}
	if r.Sys == "/sys" {
		if ring, err := QueryEthtool(ifname); err == nil {
			return ring, true
		}
	}
	return Ring{}, false
}

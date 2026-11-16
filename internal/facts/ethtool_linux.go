//go:build linux

package facts

import "github.com/safchain/ethtool"

// QueryEthtool reads channel and ring sizes through the ethtool ioctl.
// Errors (unsupported driver, no permission, virtual device) are expected;
// partial results are returned alongside a nil error when at least one query worked.
func QueryEthtool(ifname string) (Ring, error) {
	e, err := ethtool.NewEthtool()
	if err != nil {
		return Ring{}, err
	}
	defer e.Close()
	var out Ring
	ch, cerr := e.GetChannels(ifname)
	if cerr == nil {
		out.Combined, out.CombinedMax = int(ch.CombinedCount), int(ch.MaxCombined)
	}
	rg, rerr := e.GetRing(ifname)
	if rerr == nil {
		out.RxRing, out.RxRingMax = int(rg.RxPending), int(rg.RxMaxPending)
		out.TxRing, out.TxRingMax = int(rg.TxPending), int(rg.TxMaxPending)
	}
	if cerr != nil && rerr != nil {
		return Ring{}, cerr
	}
	return out, nil
}

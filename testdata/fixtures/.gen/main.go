// Command gen writes the shared fixtures under testdata/fixtures.
//
// Run from the repository root:
//
//	go run ./testdata/fixtures/.gen
//
// (Go ignores testdata, so this is built only on explicit request.) The
// output is committed; regenerate only when amending the generator. Fixtures
// are synthetic: realistic and internally consistent (cpuinfo, topology and
// NUMA agree; PCI addresses, sysfs value formats and /proc layouts follow the
// real kernel), but not captured from a particular machine.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type fx struct {
	name, desc, extends string
	files, links        map[string]string
	values, rates       map[string]float64
	nextInode           int
}

func newFx(name, desc, extends string) *fx {
	return &fx{name: name, desc: desc, extends: extends, files: map[string]string{}, links: map[string]string{},
		values: map[string]float64{}, rates: map[string]float64{}, nextInode: 20000}
}

func (f *fx) w(path, content string)  { f.files[path] = content }
func (f *fx) wl(path, content string) { f.files[path] = content + "\n" }
func (f *fx) ln(path, target string)  { f.links[path] = target }

func (f *fx) sysctl(kv map[string]string) {
	for k, v := range kv {
		f.wl("proc/sys/"+strings.ReplaceAll(k, ".", "/"), v)
	}
}

func jstr(s string) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(s)
	return strings.TrimSpace(b.String())
}

func (f *fx) emit(root string) {
	var b strings.Builder
	fmt.Fprintf(&b, "description: %s\nsynthetic: true\n", jstr(f.desc))
	if f.extends != "" {
		fmt.Fprintf(&b, "extends: %s\n", f.extends)
	}
	section := func(title string, m map[string]string) {
		if len(m) == 0 {
			return
		}
		keys := sortedKeys(m)
		fmt.Fprintf(&b, "%s:\n", title)
		for _, k := range keys {
			fmt.Fprintf(&b, "  %s: %s\n", jstr(k), jstr(m[k]))
		}
	}
	section("files", f.files)
	section("symlinks", f.links)
	fl := func(title string, m map[string]float64) {
		if len(m) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s:\n", title)
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  %s: %s\n", jstr(k), strconv.FormatFloat(m[k], 'g', -1, 64))
		}
	}
	fl("values", f.values)
	fl("rates", f.rates)
	dir := filepath.Join(root, f.name)
	must(os.MkdirAll(dir, 0o755))
	must(os.WriteFile(filepath.Join(dir, "fixture.yaml"), []byte(b.String()), 0o644))
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// ---------- CPU / NUMA / memory ----------

const (
	flagsBase   = "fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush dts acpi mmx fxsr sse sse2 ss ht tm pbe syscall nx pdpe1gb rdtscp lm constant_tsc art arch_perfmon pebs bts rep_good nopl xtopology nonstop_tsc cpuid aperfmperf pni pclmulqdq dtes64 monitor ds_cpl vmx smx est tm2 ssse3 sdbg fma cx16 xtpr pdcm pcid dca sse4_1 sse4_2 x2apic movbe popcnt tsc_deadline_timer aes xsave avx f16c rdrand lahf_lm abm 3dnowprefetch cpuid_fault epb invpcid_single ssbd ibrs ibpb stibp tpr_shadow flexpriority ept vpid ept_ad fsgsbase tsc_adjust bmi1 avx2 smep bmi2 erms invpcid xsaveopt dtherm ida arat pln pts md_clear flush_l1d"
	flagsAVX512 = " avx512f avx512dq rdseed adx smap avx512ifma clflushopt clwb intel_pt avx512cd sha_ni avx512bw avx512vl xsavec xgetbv1 xsaves avx_vnni avx512vbmi umip pku ospke waitpkg avx512_vbmi2 gfni vaes vpclmulqdq avx512_vnni avx512_bitalg avx512_vpopcntdq la57 rdpid fsrm"
	flagsBF16   = " avx512_bf16"
	flagsAMX    = " serialize tsxldtrk pconfig arch_lbr ibt amx_bf16 avx512_fp16 amx_tile amx_int8"
	flagsAMD    = "fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush mmx fxsr sse sse2 ht syscall nx mmxext fxsr_opt pdpe1gb rdtscp lm constant_tsc rep_good amd_lbr_v2 nopl nonstop_tsc cpuid extd_apicid aperfmperf rapl pni pclmulqdq monitor ssse3 fma cx16 pcid sse4_1 sse4_2 x2apic movbe popcnt aes xsave avx f16c rdrand lahf_lm cmp_legacy svm extapic cr8_legacy abm sse4a misalignsse 3dnowprefetch osvw ibs skinit wdt tce topoext perfctr_core perfctr_nb bpext perfctr_llc mwaitx cpb cat_l3 cdp_l3 invpcid_single hw_pstate ssbd mba perfmon_v2 ibrs ibpb stibp vmmcall fsgsbase bmi1 avx2 smep bmi2 erms invpcid cqm rdt_a avx512f avx512dq rdseed adx smap avx512ifma clflushopt clwb avx512cd sha_ni avx512bw avx512vl xsaveopt xsavec xgetbv1 xsaves cqm_llc cqm_occup_llc cqm_mbm_total cqm_mbm_local avx512_bf16 clzero irperf xsaveerptr rdpru wbnoinvd amd_ppin cppc amd_ibpb_ret arat npt lbrv svm_lock nrip_save tsc_scale vmcb_clean flushbyasid decodeassists pausefilter pfthreshold avic v_vmsave_vmload vgif x2avic v_spec_ctrl avx512vbmi umip pku ospke avx512_vbmi2 gfni vaes vpclmulqdq avx512_vnni avx512_bitalg avx512_vpopcntdq la57 rdpid overflow_recov succor smca fsrm flush_l1d"
)

type cpuSpec struct {
	vendor, model          string
	family, modelNum, step int
	sockets, cores, tpc    int
	flags                  string
	mhz                    string
	cacheKB                int
	hyper                  bool
}

func (c cpuSpec) total() int { return c.sockets * c.cores * c.tpc }

func (c cpuSpec) loc(i int) (sock, core, thread int) {
	per := c.sockets * c.cores
	thread = i / per
	w := i % per
	return w / c.cores, w % c.cores, thread
}

func (c cpuSpec) onSocket(s int) []int {
	var out []int
	for i := 0; i < c.total(); i++ {
		if sk, _, _ := c.loc(i); sk == s {
			out = append(out, i)
		}
	}
	return out
}

func cpuList(ids []int) string {
	var parts []string
	for i := 0; i < len(ids); {
		j := i
		for j+1 < len(ids) && ids[j+1] == ids[j]+1 {
			j++
		}
		if i == j {
			parts = append(parts, strconv.Itoa(ids[i]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", ids[i], ids[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ",")
}

func (f *fx) cpu(c cpuSpec) {
	flags := c.flags
	if c.hyper {
		flags += " hypervisor"
	}
	var b strings.Builder
	n := c.total()
	for i := 0; i < n; i++ {
		sock, core, _ := c.loc(i)
		fmt.Fprintf(&b, "processor\t: %d\nvendor_id\t: %s\n", i, c.vendor)
		if i == 0 {
			fmt.Fprintf(&b, "cpu family\t: %d\nmodel\t\t: %d\nmodel name\t: %s\nstepping\t: %d\ncpu MHz\t\t: %s\ncache size\t: %d KB\n",
				c.family, c.modelNum, c.model, c.step, c.mhz, c.cacheKB)
		} else {
			fmt.Fprintf(&b, "model name\t: %s\n", c.model)
		}
		fmt.Fprintf(&b, "physical id\t: %d\nsiblings\t: %d\ncore id\t\t: %d\ncpu cores\t: %d\n", sock, c.cores*c.tpc, core, c.cores)
		if i == 0 {
			fmt.Fprintf(&b, "flags\t\t: %s\n", flags)
		}
		b.WriteString("\n")
	}
	f.w("proc/cpuinfo", b.String())
	per := c.sockets * c.cores
	for i := 0; i < n; i++ {
		sock, core, _ := c.loc(i)
		var sib []int
		for t := 0; t < c.tpc; t++ {
			sib = append(sib, (i%per)+t*per)
		}
		top := fmt.Sprintf("sys/devices/system/cpu/cpu%d/topology/", i)
		f.wl(top+"physical_package_id", strconv.Itoa(sock))
		f.wl(top+"core_id", strconv.Itoa(core))
		f.wl(top+"thread_siblings_list", cpuList(sib))
	}
	if c.tpc > 1 {
		f.wl("sys/devices/system/cpu/smt/active", "1")
		f.wl("sys/devices/system/cpu/smt/control", "on")
	} else {
		f.wl("sys/devices/system/cpu/smt/active", "0")
		f.wl("sys/devices/system/cpu/smt/control", "notsupported")
	}
}

func (f *fx) cpufreq(driver, governor, epp, idle string) {
	d := "sys/devices/system/cpu/cpu0/cpufreq/"
	f.wl(d+"scaling_driver", driver)
	f.wl(d+"scaling_governor", governor)
	if epp != "" {
		f.wl(d+"energy_performance_preference", epp)
	}
	if idle != "" {
		f.wl("sys/devices/system/cpu/cpuidle/current_driver", idle)
	}
}

// numa writes one node per socket (nodes==sockets) or a single node.
func (f *fx) numa(c cpuSpec, nodes int, totalKB uint64) {
	for n := 0; n < nodes; n++ {
		var ids []int
		if nodes == 1 {
			for i := 0; i < c.total(); i++ {
				ids = append(ids, i)
			}
		} else {
			ids = c.onSocket(n)
		}
		d := fmt.Sprintf("sys/devices/system/node/node%d/", n)
		f.wl(d+"cpulist", cpuList(ids))
		nodeKB := totalKB / uint64(nodes)
		f.w(d+"meminfo", fmt.Sprintf("Node %d MemTotal:       %d kB\nNode %d MemFree:        %d kB\nNode %d MemUsed:        %d kB\n",
			n, nodeKB, n, nodeKB*9/10, n, nodeKB/10))
	}
}

func (f *fx) mem(totalKB, swapKB uint64, thpEnabled, thpDefrag string, hp2M int) {
	f.w("proc/meminfo", fmt.Sprintf(`MemTotal:       %d kB
MemFree:        %d kB
MemAvailable:   %d kB
Buffers:         %d kB
Cached:         %d kB
SwapCached:            0 kB
Active:         %d kB
Inactive:       %d kB
SwapTotal:      %d kB
SwapFree:       %d kB
Dirty:               412 kB
Writeback:             0 kB
AnonPages:      %d kB
Mapped:         %d kB
Shmem:             10240 kB
Slab:           %d kB
PageTables:       %d kB
HugePages_Total:       %d
HugePages_Free:        %d
HugePages_Rsvd:        0
HugePages_Surp:        0
Hugepagesize:       2048 kB
Hugetlb:        %d kB
`, totalKB, totalKB*7/10, totalKB*8/10, totalKB/200, totalKB/10, totalKB/8, totalKB/12, swapKB, swapKB,
		totalKB/20, totalKB/60, totalKB/80, totalKB/700, hp2M, hp2M, hp2M*2048))
	f.wl("sys/kernel/mm/transparent_hugepage/enabled", bracket([]string{"always", "madvise", "never"}, thpEnabled))
	f.wl("sys/kernel/mm/transparent_hugepage/defrag", bracket([]string{"always", "defer", "defer+madvise", "madvise", "never"}, thpDefrag))
	f.wl("sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages", strconv.Itoa(hp2M))
	f.wl("sys/kernel/mm/hugepages/hugepages-1048576kB/nr_hugepages", "0")
}

func bracket(opts []string, active string) string {
	out := make([]string, len(opts))
	for i, o := range opts {
		if o == active {
			o = "[" + o + "]"
		}
		out[i] = o
	}
	return strings.Join(out, " ")
}

// ---------- devices ----------

type nicSpec struct {
	name, driver, vendor string
	speed, mtu, numa     int
	queues               int
	ring                 *ringSpec // nil: no ethtool data
	noSpeed              bool
}

type ringSpec struct{ comb, combMax, rx, rxMax, tx, txMax int }

func (f *fx) nic(n nicSpec) {
	d := "sys/class/net/" + n.name + "/"
	f.wl(d+"mtu", strconv.Itoa(n.mtu))
	f.wl(d+"operstate", "up")
	if n.driver != "" {
		f.wl(d+"device/vendor", n.vendor)
		f.wl(d+"device/numa_node", strconv.Itoa(n.numa))
		f.ln(d+"device/driver", "../../../bus/pci/drivers/"+n.driver)
	}
	if n.speed > 0 && !n.noSpeed {
		f.wl(d+"speed", strconv.Itoa(n.speed))
	}
	for q := 0; q < n.queues; q++ {
		f.wl(fmt.Sprintf("%squeues/rx-%d/rps_cpus", d, q), "0")
		f.wl(fmt.Sprintf("%squeues/tx-%d/xps_cpus", d, q), "0")
	}
	if r := n.ring; r != nil {
		f.wl("run/kerneltuner-fixture/ethtool/"+n.name+".json",
			fmt.Sprintf(`{"combined":%d,"combined_max":%d,"rx_ring":%d,"rx_ring_max":%d,"tx_ring":%d,"tx_ring_max":%d}`,
				r.comb, r.combMax, r.rx, r.rxMax, r.tx, r.txMax))
	}
}

func (f *fx) block(name string, rotational bool, sched string, schedulers []string, ra, nr int) {
	d := "sys/block/" + name + "/"
	rot := "0"
	if rotational {
		rot = "1"
	}
	f.wl(d+"queue/rotational", rot)
	f.wl(d+"queue/scheduler", bracket(schedulers, sched))
	f.wl(d+"queue/read_ahead_kb", strconv.Itoa(ra))
	f.wl(d+"queue/nr_requests", strconv.Itoa(nr))
	if strings.HasPrefix(name, "nvme") {
		f.wl(d+"device/transport", "pcie")
	}
}

type pciSpec struct {
	addr, vendor, device, class, driver string
	numa                                int
	speed, maxSpeed                     string
	width, maxWidth                     int
	group                               int
}

func (f *fx) pci(p pciSpec) {
	d := "sys/bus/pci/devices/" + p.addr + "/"
	f.wl(d+"vendor", p.vendor)
	f.wl(d+"device", p.device)
	f.wl(d+"class", p.class)
	f.wl(d+"numa_node", strconv.Itoa(p.numa))
	if p.speed != "" {
		f.wl(d+"current_link_speed", p.speed)
		f.wl(d+"max_link_speed", p.maxSpeed)
		f.wl(d+"current_link_width", strconv.Itoa(p.width))
		f.wl(d+"max_link_width", strconv.Itoa(p.maxWidth))
	}
	f.ln(d+"driver", "../../../bus/pci/drivers/"+p.driver)
	if p.group > 0 {
		f.ln(d+"iommu_group", "../../../kernel/iommu_groups/"+strconv.Itoa(p.group))
	}
}

func (f *fx) iommu(unit string, groups []int, typ string) {
	switch unit {
	case "dmar0":
		f.wl("sys/class/iommu/dmar0/intel-iommu/version", "1:0")
	default:
		f.wl("sys/class/iommu/"+unit+"/amd-iommu/cap", "40")
	}
	for _, g := range groups {
		f.wl(fmt.Sprintf("sys/kernel/iommu_groups/%d/type", g), typ)
	}
}

func (f *fx) rdma(name, state, layer, rate, netdev string) {
	d := "sys/class/infiniband/" + name + "/"
	f.wl(d+"ports/1/state", state)
	f.wl(d+"ports/1/link_layer", layer)
	f.wl(d+"ports/1/rate", rate)
	if netdev != "" {
		f.wl(d+"device/net/"+netdev+"/mtu", "4200")
	}
}

// ---------- processes ----------

type procSpec struct {
	pid                  int
	comm                 string
	argv                 []string
	nofile, nofileHard   string
	memlock, memlockHard string
	unit                 string
	fds                  int
}

func (f *fx) proc(p procSpec) {
	d := fmt.Sprintf("proc/%d/", p.pid)
	f.wl(d+"comm", p.comm)
	if len(p.argv) > 0 {
		f.w(d+"cmdline", strings.Join(p.argv, "\x00")+"\x00")
	} else {
		f.w(d+"cmdline", "")
	}
	nf, nfh := orDef(p.nofile, "1024"), orDef(p.nofileHard, "1048576")
	ml, mlh := orDef(p.memlock, "8388608"), orDef(p.memlockHard, "8388608")
	f.w(d+"limits", fmt.Sprintf(`Limit                     Soft Limit           Hard Limit           Units
Max cpu time              unlimited            unlimited            seconds
Max file size             unlimited            unlimited            bytes
Max data size             unlimited            unlimited            bytes
Max stack size            8388608              unlimited            bytes
Max core file size        0                    unlimited            bytes
Max resident set          unlimited            unlimited            bytes
Max processes             514380               514380               processes
%-25s %-20s %-20s %-10s
Max locked memory         %-20s %-20s %-10s
Max address space         unlimited            unlimited            bytes
`, "Max open files", nf, nfh, "files", ml, mlh, "bytes"))
	if p.unit != "" {
		f.w(d+"cgroup", "0::/system.slice/"+p.unit+"\n")
	} else {
		f.w(d+"cgroup", "0::/user.slice/user-1000.slice/session-3.scope\n")
	}
	for i := 0; i < p.fds; i++ {
		f.w(fmt.Sprintf("%sfd/%d", d, i), "")
	}
}

func orDef(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// listeners writes /proc/net/tcp with LISTEN sockets on ports (0.0.0.0).
func (f *fx) listeners(ports ...int) {
	var b strings.Builder
	b.WriteString("  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")
	for i, p := range ports {
		f.nextInode++
		fmt.Fprintf(&b, "%4d: 00000000:%04X 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 %d 1 0000000000000000 100 0 0 10 0\n",
			i, p, f.nextInode)
	}
	f.w("proc/net/tcp", b.String())
}

func (f *fx) modules(names ...string) {
	var b strings.Builder
	for i, n := range names {
		fmt.Fprintf(&b, "%s %d %d - Live 0xffffffffc0%03x000\n", n, 16384+i*4096, i%3, i)
	}
	f.w("proc/modules", b.String())
}

func (f *fx) dmi(vendor, product, bios string) {
	f.wl("sys/class/dmi/id/sys_vendor", vendor)
	f.wl("sys/class/dmi/id/product_name", product)
	f.wl("sys/class/dmi/id/bios_vendor", bios)
}

func (f *fx) kernel(release, cmdline string) {
	f.wl("proc/sys/kernel/osrelease", release)
	f.wl("proc/cmdline", cmdline)
}

func (f *fx) fileNr(allocated, max int) {
	f.wl("proc/sys/fs/file-nr", fmt.Sprintf("%d\t0\t%d", allocated, max))
	f.wl("proc/sys/fs/file-max", strconv.Itoa(max))
}

var (
	schedMQ     = []string{"none", "mq-deadline"}
	schedLegacy = []string{"noop", "deadline", "cfq"}
)

func systemd(pid int) procSpec {
	return procSpec{pid: pid, comm: "systemd", argv: []string{"/sbin/init"}, nofile: "1024", nofileHard: "524288",
		memlock: "8388608", memlockHard: "8388608", fds: 12}
}

func sshd(pid int) procSpec {
	return procSpec{pid: pid, comm: "sshd", argv: []string{"sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups"},
		nofile: "1024", nofileHard: "524288", unit: "ssh.service", fds: 5}
}

func main() {
	root := "testdata/fixtures"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	for _, f := range []*fx{
		base(), web(), legacy(), k8sRedis(), h100(), mi300x(), gaudi3(), intelMax(), amx(), bpftuneTuned(),
	} {
		f.emit(root)
	}
}

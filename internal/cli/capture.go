package cli

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts/accel"
)

const (
	maxCaptureFile = 1 << 20 // bytes read per file
	maxSysctlFile  = 4096
	maxFDEntries   = 256 // fd/ placeholders per process (the collector only counts entries)
)

func newCaptureCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Archive /proc and /sys data for fixtures and bug reports",
		Long: `Capture reads an explicit allowlist of /proc, /sys and /etc paths (see
docs/DESIGN.md) and writes them as a tar.gz archive, or with --fixture as a
fixture.yaml (synthetic: false) that can be dropped into testdata/fixtures.

Privacy: only the process name plus argv[0..1] is kept from each command line
(secrets and host names redacted); environ, fd targets, host names, serials and
MAC/IP addresses are never read.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, _ := cmd.Flags().GetString("output")
			asFixture, _ := cmd.Flags().GetBool("fixture")
			if out == "" && !asFixture {
				return errors.New("capture: -o FILE is required (use -o - for stdout)")
			}
			snap := captureRoot(g.Root())
			w, closeFn, err := openOutput(cmd, out)
			if err != nil {
				return err
			}
			if asFixture {
				err = snap.writeFixture(w)
			} else {
				err = snap.writeArchive(w)
			}
			if cerr := closeFn(); err == nil {
				err = cerr
			}
			return err
		},
	}
	cmd.Flags().StringP("output", "o", "", "output file (- or empty with --fixture: stdout)")
	cmd.Flags().Bool("fixture", false, "emit a fixture.yaml instead of an archive")
	return cmd
}

func openOutput(cmd *cobra.Command, out string) (io.Writer, func() error, error) {
	if out == "" || out == "-" || out == "/dev/stdout" {
		return cmd.OutOrStdout(), func() error { return nil }, nil
	}
	f, err := os.Create(out)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}

// snapshot is the sanitized allowlisted view of a root: fixture-relative path
// -> content, and symlinks (path -> target).
type snapshot struct {
	Release  string
	Files    map[string]string
	Symlinks map[string]string
	Warnings []string
}

type capturer struct {
	r      facts.Root
	snap   *snapshot
	hosts  []string
	hostRE *regexp.Regexp
}

// captureRoot snapshots the allowlisted files under r.
func captureRoot(r facts.Root) *snapshot {
	c := &capturer{r: r, snap: &snapshot{Files: map[string]string{}, Symlinks: map[string]string{}}}
	c.initHosts()
	c.proc()
	c.sysctl()
	c.procs()
	c.sys()
	c.misc()
	c.ethtool()
	c.snap.Release = strings.TrimSpace(c.snap.Files["proc/sys/kernel/osrelease"])
	return c.snap
}

func (c *capturer) initHosts() {
	seen := map[string]bool{}
	add := func(h string) {
		h = strings.TrimSpace(h)
		if len(h) >= 3 && !seen[strings.ToLower(h)] {
			seen[strings.ToLower(h)] = true
			c.hosts = append(c.hosts, h)
			if short, _, ok := strings.Cut(h, "."); ok && len(short) >= 3 && !seen[strings.ToLower(short)] {
				seen[strings.ToLower(short)] = true
				c.hosts = append(c.hosts, short)
			}
		}
	}
	if h, err := os.Hostname(); err == nil {
		add(h)
	}
	if b, err := os.ReadFile(filepath.Join(c.r.Proc, "sys/kernel/hostname")); err == nil {
		add(string(b))
	}
	if len(c.hosts) == 0 {
		return
	}
	quoted := make([]string, len(c.hosts))
	for i, h := range c.hosts {
		quoted[i] = regexp.QuoteMeta(h)
	}
	c.hostRE = regexp.MustCompile(`(?i)` + strings.Join(quoted, "|"))
}

func (c *capturer) scrub(s string) string {
	if c.hostRE != nil {
		s = c.hostRE.ReplaceAllString(s, "redacted-host")
	}
	return s
}

func readLimited(path string, limit int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, limit))
	return string(b), err
}

type transform func(string) string

// file stores base/rel under key prefix/rel; missing or unreadable files are skipped.
func (c *capturer) file(prefix, base, rel string, tf transform) {
	s, err := readLimited(filepath.Join(base, rel), maxCaptureFile)
	if err != nil {
		return
	}
	if tf != nil {
		s = tf(s)
	}
	c.snap.Files[prefix+"/"+filepath.ToSlash(rel)] = c.scrub(s)
}

// glob stores every regular file matching pattern (relative to base).
func (c *capturer) glob(prefix, base, pattern string, tf transform) {
	matches, _ := filepath.Glob(filepath.Join(base, pattern))
	for _, m := range matches {
		rel, err := filepath.Rel(base, m)
		if err != nil {
			continue
		}
		if st, err := os.Stat(m); err != nil || !st.Mode().IsRegular() {
			continue
		}
		c.file(prefix, base, rel, tf)
	}
}

// symlinks stores every symlink matching pattern with its raw target.
func (c *capturer) symlinks(prefix, base, pattern string) {
	matches, _ := filepath.Glob(filepath.Join(base, pattern))
	for _, m := range matches {
		t, err := os.Readlink(m)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(base, m)
		if err != nil {
			continue
		}
		c.snap.Symlinks[prefix+"/"+filepath.ToSlash(rel)] = t
	}
}

// marker stores an empty placeholder file.
func (c *capturer) marker(key string) { c.snap.Files[key] = "" }

func (c *capturer) proc() {
	p := c.r.Proc
	for _, rel := range []string{"sys/kernel/osrelease", "meminfo", "modules", "sys/fs/file-nr", "vmstat",
		"pressure/cpu", "pressure/memory", "pressure/io", "net/netstat", "net/snmp", "net/softnet_stat", "net/sockstat", "1/cgroup"} {
		c.file("proc", p, rel, nil)
	}
	c.file("proc", p, "cmdline", redactCmdlineText)
	c.file("proc", p, "cpuinfo", trimCPUInfo)
	c.file("proc", p, "net/tcp", listenOnly)
	c.file("proc", p, "net/tcp6", listenOnly)
	// Only the container= entry of pid 1's environment is kept.
	if s, err := readLimited(filepath.Join(p, "1/environ"), maxCaptureFile); err == nil {
		for _, kv := range strings.Split(s, "\x00") {
			if strings.HasPrefix(kv, "container=") {
				c.snap.Files["proc/1/environ"] = kv + "\x00"
			}
		}
	}
}

var sysctlSkip = map[string]bool{
	"hostname": true, "domainname": true, "tcp_fastopen_key": true, "stat_refresh": true,
}

func (c *capturer) sysctl() {
	base := filepath.Join(c.r.Proc, "sys")
	for _, d := range facts.SysctlDirs {
		ents, err := os.ReadDir(filepath.Join(base, d))
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.Type().IsRegular() || sysctlSkip[e.Name()] {
				continue
			}
			if info, err := e.Info(); err != nil || info.Mode().Perm()&0o444 == 0 {
				continue
			}
			s, err := readLimited(filepath.Join(base, d, e.Name()), maxSysctlFile)
			if err != nil {
				continue
			}
			c.snap.Files["proc/sys/"+d+"/"+e.Name()] = c.scrub(s)
		}
	}
}

var (
	reIPv4   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	reUUID   = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	reEmail  = regexp.MustCompile(`\S+@\S+`)
	reFQDN   = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9-]*(?:\.[a-z0-9-]+)*\.(?:com|net|org|io|dev|cloud|internal|local|lan|corp|home|co|us|uk|de|eu)\b`)
	reSecret = regexp.MustCompile(`(?i)^(-{0,2}[a-z_-]*(?:pass|secret|token|key|auth|cred)[a-z_-]*)=.*$`)
)

func redactArg(a string) string {
	if reSecret.MatchString(a) {
		return reSecret.ReplaceAllString(a, "$1=[redacted]")
	}
	a = reEmail.ReplaceAllString(a, "[redacted]")
	a = reUUID.ReplaceAllString(a, "[uuid]")
	a = reIPv4.ReplaceAllStringFunc(a, func(ip string) string {
		if ip == "0.0.0.0" || strings.HasPrefix(ip, "127.") {
			return ip
		}
		return "[ip]"
	})
	return reFQDN.ReplaceAllString(a, "[host]")
}

func redactCmdlineText(s string) string {
	fields := strings.Fields(s)
	for i, f := range fields {
		fields[i] = redactArg(f)
	}
	return strings.Join(fields, " ") + "\n"
}

// keepArgv keeps argv[0..1] of a NUL-separated command line, redacted.
func keepArgv(raw string) string {
	raw = strings.TrimRight(raw, "\x00")
	if raw == "" {
		return ""
	}
	args := strings.Split(raw, "\x00")
	if len(args) > 2 {
		args = args[:2]
	}
	for i, a := range args {
		args[i] = redactArg(a)
	}
	if strings.HasPrefix(args[0], "sshd:") { // process title carries the login name
		args = []string{"sshd: [redacted]"}
	}
	return strings.Join(args, "\x00") + "\x00"
}

// trimCPUInfo keeps the first processor block whole and only the topology
// lines of the others, so fixtures stay small.
func trimCPUInfo(s string) string {
	var b strings.Builder
	first := true
	for _, blk := range strings.Split(s, "\n\n") {
		if strings.TrimSpace(blk) == "" {
			continue
		}
		if first {
			b.WriteString(blk + "\n\n")
			first = false
			continue
		}
		for _, line := range strings.Split(blk, "\n") {
			k, _, _ := strings.Cut(line, ":")
			switch strings.TrimSpace(k) {
			case "processor", "vendor_id", "model name", "physical id", "siblings", "core id", "cpu cores", "CPU implementer", "CPU part":
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// listenOnly keeps the header and LISTEN (state 0A) sockets, with local
// addresses zeroed so no host IPs are recorded.
func listenOnly(s string) string {
	var b strings.Builder
	for i, line := range strings.Split(s, "\n") {
		if i == 0 {
			b.WriteString(line + "\n")
			continue
		}
		f := strings.Fields(line)
		if len(f) < 4 || f[3] != "0A" {
			continue
		}
		host, port, ok := strings.Cut(f[1], ":")
		if !ok {
			continue
		}
		f[1] = strings.Repeat("0", len(host)) + ":" + port
		b.WriteString(strings.Join(f, " ") + "\n")
	}
	return b.String()
}

// trimLimits keeps the header and the two limits the collector reads.
func trimLimits(s string) string {
	var b strings.Builder
	for i, line := range strings.Split(s, "\n") {
		if i == 0 || strings.HasPrefix(line, "Max open files") || strings.HasPrefix(line, "Max locked memory") {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

var reHexID = regexp.MustCompile(`[0-9a-f]{12,64}`)

// scrubCgroup replaces container IDs; unit and slice names stay.
func scrubCgroup(s string) string { return reHexID.ReplaceAllString(s, "[id]") }

func (c *capturer) procs() {
	ents, err := os.ReadDir(c.r.Proc)
	if err != nil {
		c.snap.Warnings = append(c.snap.Warnings, err.Error())
		return
	}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		dir := filepath.Join(c.r.Proc, e.Name())
		comm, err := readLimited(filepath.Join(dir, "comm"), 64)
		if err != nil {
			continue
		}
		raw, _ := readLimited(filepath.Join(dir, "cmdline"), 16384)
		if strings.Trim(raw, "\x00") == "" {
			continue // kernel thread or zombie: irrelevant to tuning, and noisy
		}
		key := "proc/" + strconv.Itoa(pid) + "/"
		c.snap.Files[key+"comm"] = c.scrub(comm)
		c.snap.Files[key+"cmdline"] = c.scrub(keepArgv(raw))
		c.file("proc", c.r.Proc, e.Name()+"/limits", trimLimits)
		c.file("proc", c.r.Proc, e.Name()+"/cgroup", scrubCgroup)
		// Only the number of open fds is recorded, never their targets.
		if fds, err := os.ReadDir(filepath.Join(dir, "fd")); err == nil {
			for i := 0; i < len(fds) && i < maxFDEntries; i++ {
				c.marker(key + "fd/" + strconv.Itoa(i))
			}
		}
	}
}

func (c *capturer) sys() {
	s := c.r.Sys
	for _, rel := range []string{"sys_vendor", "product_name", "bios_vendor", "chassis_asset_tag"} {
		c.file("sys", s, "class/dmi/id/"+rel, nil)
	}
	cpu := "devices/system/cpu/"
	c.glob("sys", s, cpu+"cpu[0-9]*/topology/physical_package_id", nil)
	c.glob("sys", s, cpu+"cpu[0-9]*/topology/core_id", nil)
	c.glob("sys", s, cpu+"cpu[0-9]*/topology/thread_siblings_list", nil)
	for _, f := range []string{"scaling_driver", "scaling_governor", "energy_performance_preference"} {
		c.file("sys", s, cpu+"cpu0/cpufreq/"+f, nil)
	}
	c.file("sys", s, cpu+"smt/active", nil)
	c.file("sys", s, cpu+"smt/control", nil)
	c.file("sys", s, cpu+"cpuidle/current_driver", nil)
	c.glob("sys", s, "devices/system/node/node[0-9]*/cpulist", nil)
	c.glob("sys", s, "devices/system/node/node[0-9]*/meminfo", nil)
	c.file("sys", s, "kernel/mm/transparent_hugepage/enabled", nil)
	c.file("sys", s, "kernel/mm/transparent_hugepage/defrag", nil)
	c.glob("sys", s, "kernel/mm/hugepages/hugepages-*/nr_hugepages", nil)

	c.nets()
	c.blocks()
	c.pci()

	c.glob("sys", s, "class/kfd/kfd/topology/nodes/*/properties", nil)
	c.glob("sys", s, "class/accel/accel[0-9]*/device/vendor", nil)
	c.glob("sys", s, "class/accel/accel[0-9]*/device/device", nil)
	c.glob("sys", s, "class/infiniband/*/ports/*/state", nil)
	c.glob("sys", s, "class/infiniband/*/ports/*/link_layer", nil)
	c.glob("sys", s, "class/infiniband/*/ports/*/rate", nil)
	// Only the names of the netdevs behind an RDMA device are recorded.
	nets, _ := filepath.Glob(filepath.Join(s, "class/infiniband/*/device/net/*"))
	for _, n := range nets {
		if rel, err := filepath.Rel(s, n); err == nil {
			c.file("sys", s, rel+"/mtu", nil)
			if _, ok := c.snap.Files["sys/"+filepath.ToSlash(rel)+"/mtu"]; !ok {
				c.marker("sys/" + filepath.ToSlash(rel) + "/mtu")
			}
		}
	}
	c.glob("sys", s, "class/iommu/*/*/version", nil)
	c.glob("sys", s, "class/iommu/*/*/cap", nil)
	c.glob("sys", s, "kernel/iommu_groups/*/type", nil)
}

func (c *capturer) nets() {
	s := c.r.Sys
	ents, err := os.ReadDir(filepath.Join(s, "class/net"))
	if err != nil {
		return
	}
	for _, e := range ents {
		n := e.Name()
		if n == "lo" {
			continue
		}
		base := "class/net/" + n + "/"
		c.file("sys", s, base+"mtu", nil)
		c.file("sys", s, base+"speed", nil)
		hasDev := false
		if _, err := os.Stat(filepath.Join(s, base, "device")); err == nil {
			hasDev = true
		}
		if hasDev {
			c.file("sys", s, base+"device/vendor", nil)
			c.file("sys", s, base+"device/numa_node", nil)
			c.symlinks("sys", s, base+"device/driver")
			// Make sure the device/ directory exists even without a vendor file.
			if _, ok := c.snap.Files["sys/"+base+"device/vendor"]; !ok && len(c.snap.Symlinks["sys/"+base+"device/driver"]) == 0 {
				c.file("sys", s, base+"device/uevent", nil)
			}
		}
		qs, _ := os.ReadDir(filepath.Join(s, base, "queues"))
		for _, q := range qs {
			fname := ""
			switch {
			case strings.HasPrefix(q.Name(), "rx-"):
				fname = "rps_cpus"
			case strings.HasPrefix(q.Name(), "tx-"):
				fname = "xps_cpus"
			default:
				continue
			}
			c.file("sys", s, base+"queues/"+q.Name()+"/"+fname, nil)
			// The collector counts queue directories: keep them even if the file is unreadable.
			key := "sys/" + base + "queues/" + q.Name() + "/" + fname
			if _, ok := c.snap.Files[key]; !ok {
				c.marker(key)
			}
		}
	}
}

func (c *capturer) blocks() {
	s := c.r.Sys
	ents, err := os.ReadDir(filepath.Join(s, "block"))
	if err != nil {
		return
	}
	for _, e := range ents {
		if facts.SkipBlock(e.Name()) {
			continue
		}
		base := "block/" + e.Name() + "/"
		for _, f := range []string{"rotational", "scheduler", "read_ahead_kb", "nr_requests"} {
			c.file("sys", s, base+"queue/"+f, nil)
		}
		c.file("sys", s, base+"device/transport", nil)
	}
}

func (c *capturer) pci() {
	s := c.r.Sys
	ents, err := os.ReadDir(filepath.Join(s, "bus/pci/devices"))
	if err != nil {
		return
	}
	for _, e := range ents {
		base := "bus/pci/devices/" + e.Name() + "/"
		vendor, _ := readLimited(filepath.Join(s, base, "vendor"), 32)
		class, _ := readLimited(filepath.Join(s, base, "class"), 32)
		if accel.Classify(vendor, class) == "" {
			continue
		}
		for _, f := range []string{"vendor", "device", "class", "numa_node", "current_link_speed", "max_link_speed", "current_link_width", "max_link_width"} {
			c.file("sys", s, base+f, nil)
		}
		c.symlinks("sys", s, base+"driver")
		c.symlinks("sys", s, base+"iommu_group")
	}
}

func (c *capturer) misc() {
	c.file("etc", c.r.Etc, "tuned/active_profile", nil)
	if exists(filepath.Join(c.r.Run, ".containerenv")) {
		c.marker("run/.containerenv")
	}
	if exists(filepath.Join(filepath.Dir(c.r.Etc), ".dockerenv")) {
		c.marker(".dockerenv")
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// ethtool records ring/channel sizes for physical NICs through the ioctl, in
// the fixture-only file the collector reads when Root.Sys is not /sys.
func (c *capturer) ethtool() {
	for key := range c.snap.Files {
		const pre, suf = "sys/class/net/", "/device/vendor"
		if !strings.HasPrefix(key, pre) || !strings.HasSuffix(key, suf) {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, pre), suf)
		dst := "run/kerneltuner-fixture/ethtool/" + name + ".json"
		// A fixture root already carries its ring data; copy it through.
		if b, err := os.ReadFile(facts.FixtureEthtoolPath(c.r, name)); err == nil {
			c.snap.Files[dst] = string(b)
			continue
		}
		ring, err := facts.QueryEthtool(name)
		if err != nil || ring == (facts.Ring{}) {
			continue
		}
		js := fmt.Sprintf(`{"combined":%d,"combined_max":%d,"rx_ring":%d,"rx_ring_max":%d,"tx_ring":%d,"tx_ring_max":%d}`+"\n",
			ring.Combined, ring.CombinedMax, ring.RxRing, ring.RxRingMax, ring.TxRing, ring.TxRingMax)
		c.snap.Files[dst] = js
	}
}

type fixtureDoc struct {
	Description string            `yaml:"description"`
	Synthetic   bool              `yaml:"synthetic"`
	Files       map[string]string `yaml:"files"`
	Symlinks    map[string]string `yaml:"symlinks,omitempty"`
}

func (s *snapshot) writeFixture(w io.Writer) error {
	doc := fixtureDoc{
		Description: fmt.Sprintf("Captured by kerneltuner capture, kernel %s", s.Release),
		Files:       s.Files,
		Symlinks:    s.Symlinks,
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return enc.Close()
}

func (s *snapshot) writeArchive(w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	now := time.Now()
	keys := make([]string, 0, len(s.Files))
	for k := range s.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		body := s.Files[k]
		if err := tw.WriteHeader(&tar.Header{Name: k, Mode: 0o644, Size: int64(len(body)), ModTime: now, Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := io.WriteString(tw, body); err != nil {
			return err
		}
	}
	links := make([]string, 0, len(s.Symlinks))
	for k := range s.Symlinks {
		links = append(links, k)
	}
	sort.Strings(links)
	for _, k := range links {
		if err := tw.WriteHeader(&tar.Header{Name: k, Linkname: s.Symlinks[k], Mode: 0o777, ModTime: now, Typeflag: tar.TypeSymlink}); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

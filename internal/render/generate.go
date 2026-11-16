package render

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"regexp"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// ifNameRe is what a network interface name may look like (IFNAMSIZ-1).
var ifNameRe = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,15}$`)

var shellSafeRe = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shq quotes s for a POSIX shell; plain words are left as they are.
func shq(s string) string {
	if shellSafeRe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Generated file names (paths relative to --out).
const (
	FileSysctl  = "90-kerneltuner.conf"
	FileTuned   = "tuned.conf"
	FileCmdline = "grub-cmdline.txt"
	FileNIC     = "nic.sh"
	FileRuntime = "runtime.md"
	FileLimitsD = "limits.d/90-kerneltuner.conf"
)

// Formats lists the values accepted by Generate.
var Formats = []string{"sysctl", "limits", "systemd", "tuned", "cmdline", "nic", "runtime", "all"}

// Generate renders the config files for format ("all" renders every format).
// The result maps a relative path to the file body; formats with nothing to
// say contribute no file. "limits" yields the systemd drop-ins plus the
// limits.d fallback; "systemd" yields only the drop-ins.
func Generate(format string, recs []advisor.Recommendation, m Meta) (map[string]string, error) {
	out := map[string]string{}
	add := func(name, body string) {
		if body != "" {
			out[name] = body
		}
	}
	merge := func(files map[string]string) {
		for k, v := range files {
			out[k] = v
		}
	}
	want := func(f string) bool { return format == f || format == "all" }
	known := false
	for _, f := range Formats {
		known = known || f == format
	}
	if !known {
		return nil, fmt.Errorf("unknown format %q (want one of %s)", format, strings.Join(Formats, ", "))
	}
	if want("sysctl") {
		add(FileSysctl, Sysctl(recs, m))
	}
	if want("limits") || want("systemd") {
		dropins, limitsd := Limits(recs, m)
		merge(dropins)
		if want("limits") {
			add(FileLimitsD, limitsd)
		}
	}
	if want("tuned") {
		add(FileTuned, Tuned(recs, m))
	}
	if want("cmdline") {
		add(FileCmdline, Cmdline(recs, m))
	}
	if want("nic") {
		add(FileNIC, NIC(recs, m))
	}
	if want("runtime") {
		add(FileRuntime, Runtime(recs, m))
	}
	return out, nil
}

// pick returns recommend-status results of the given kinds, in a stable order.
// Duplicates targeting the same (kind, param, target) are merged first
// (advisor.Reconcile), so each setting appears once.
func pick(recs []advisor.Recommendation, kinds ...string) []advisor.Recommendation {
	var out []advisor.Recommendation
	recs, _ = advisor.Reconcile(recs)
	for _, r := range recs {
		if r.Status != advisor.StatusRecommend {
			continue
		}
		// Values may derive from host data: no control characters in any file.
		r.Param, r.Target, r.Proposed = facts.SafeText(r.Param), facts.SafeText(r.Target), facts.SafeText(r.Proposed)
		r.RuleID, r.Title = facts.SafeText(r.RuleID), facts.SafeText(r.Title)
		for _, k := range kinds {
			if r.Kind == k {
				out = append(out, r)
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Param != out[j].Param {
			return out[i].Param < out[j].Param
		}
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

// comment is the comment block above a setting: every contributing rule id,
// the title, and every source of those rules with its quote.
func comment(r advisor.Recommendation, m Meta) string {
	ids := r.RuleID
	if len(r.Contributors) > 1 {
		ids = strings.Join(r.Contributors, ", ")
	}
	c := fmt.Sprintf("# %s: %s\n", ids, r.Title)
	if len(r.Contributors) > 1 && r.Note != "" {
		c += "# " + r.Note + "\n"
	}
	return c + sourceComments(r, m, "# ")
}

// sourceComments lists every source of r (every contributor's, when merged).
func sourceComments(r advisor.Recommendation, m Meta, prefix string) string {
	var b strings.Builder
	for _, c := range m.Cites.For(r) {
		b.WriteString(c.commentLines(prefix, len(r.Contributors) > 1))
	}
	return b.String()
}

// targets lists the items a recommendation covers: Targets for a summarized
// foreach, else the single Target.
func targets(r advisor.Recommendation) []string {
	if len(r.Targets) > 0 {
		return r.Targets
	}
	return []string{r.Target}
}

// Sysctl renders /etc/sysctl.d/90-kerneltuner.conf. Empty when nothing applies.
func Sysctl(recs []advisor.Recommendation, m Meta) string {
	rs := pick(recs, "sysctl")
	if len(rs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.header("# "))
	b.WriteString("# Install as /etc/sysctl.d/90-kerneltuner.conf, then: sysctl --system\n")
	for _, r := range rs {
		b.WriteString("\n" + comment(r, m))
		fmt.Fprintf(&b, "%s = %s\n", facts.SafeText(r.Param), facts.SafeText(r.Proposed))
	}
	return b.String()
}

func isUnit(target string) bool {
	for _, s := range []string{".service", ".scope", ".socket", ".slice"} {
		if strings.HasSuffix(target, s) {
			return true
		}
	}
	return false
}

// Limits renders systemd drop-ins (map keyed by systemd/<unit>.d/90-kerneltuner.conf)
// for apps with a known unit, and a limits.d fallback body for the others.
func Limits(recs []advisor.Recommendation, m Meta) (dropins map[string]string, limitsD string) {
	rs := pick(recs, "limit")
	dropins = map[string]string{}
	units := map[string]*strings.Builder{}
	var fb strings.Builder
	for _, r := range rs {
		if isUnit(r.Target) && facts.ValidUnitName(r.Target) {
			b := units[r.Target]
			if b == nil {
				b = &strings.Builder{}
				b.WriteString(m.header("# "))
				b.WriteString("# Then: systemctl daemon-reload && systemctl restart " + shq(r.Target) + "\n\n[Service]\n")
				units[r.Target] = b
			}
			b.WriteString(comment(r, m))
			fmt.Fprintf(b, "%s=%s\n", facts.SafeText(r.Param), facts.SafeText(r.Proposed))
			continue
		}
		if fb.Len() == 0 {
			fb.WriteString(m.header("# "))
			fb.WriteString("# Fallback for apps not run by systemd (/etc/security/limits.d/). Applies at next login.\n")
			fb.WriteString("# '*' covers all users; narrow it to the app's user.\n")
		}
		ids := r.RuleID
		if len(r.Contributors) > 1 {
			ids = strings.Join(r.Contributors, ", ")
		}
		fmt.Fprintf(&fb, "\n# %s (%s): %s\n", facts.SafeText(ids), facts.SafeText(r.Target), facts.SafeText(r.Title))
		fb.WriteString(sourceComments(r, m, "# "))
		item, val := limitsConfLine(r)
		fmt.Fprintf(&fb, "*\tsoft\t%s\t%s\n*\thard\t%s\t%s\n", item, val, item, val)
	}
	for u, b := range units {
		dropins["systemd/"+u+".d/90-kerneltuner.conf"] = b.String()
	}
	return dropins, fb.String()
}

func limitsConfLine(r advisor.Recommendation) (item, val string) {
	item = "nofile"
	switch {
	case strings.EqualFold(r.Param, "LimitMEMLOCK"):
		item = "memlock"
	case strings.EqualFold(r.Param, "LimitNPROC"):
		item = "nproc"
	}
	if r.Proposed == "infinity" || r.Proposed == "unlimited" {
		return item, "unlimited"
	}
	if item == "memlock" { // limits.conf wants KiB; systemd takes bytes
		if n, err := strconv.ParseFloat(r.Proposed, 64); err == nil {
			return item, strconv.FormatFloat(math.Ceil(n/1024), 'f', 0, 64)
		}
	}
	return item, r.Proposed
}

// Tuned renders a tuned profile (tuned.conf).
func Tuned(recs []advisor.Recommendation, m Meta) string {
	sysctls := pick(recs, "sysctl")
	sysfs := pick(recs, "sysfs")
	cmd := pick(recs, "cmdline")
	if len(sysctls)+len(sysfs)+len(cmd) == 0 {
		return ""
	}
	include := "throughput-performance"
	if m.Facts != nil && m.Facts.Tuned.Active && m.Facts.Tuned.Profile != "" {
		include = m.Facts.Tuned.Profile
	}
	var b strings.Builder
	b.WriteString(m.header("# "))
	b.WriteString("# Install as /etc/tuned/kerneltuner/tuned.conf, then: tuned-adm profile kerneltuner\n")
	fmt.Fprintf(&b, "\n[main]\ninclude=%s\n", include)
	if len(sysctls) > 0 {
		b.WriteString("\n[sysctl]\n")
		for _, r := range sysctls {
			b.WriteString(comment(r, m))
			fmt.Fprintf(&b, "%s=%s\n", r.Param, r.Proposed)
		}
	}
	if len(sysfs) > 0 {
		b.WriteString("\n[sysfs]\n")
		for _, r := range sysfs {
			b.WriteString(comment(r, m))
			for _, t := range targets(r) {
				fmt.Fprintf(&b, "%s=%s\n", strings.ReplaceAll(r.Param, "{item}", t), r.Proposed)
			}
		}
	}
	if len(cmd) > 0 {
		b.WriteString("\n[bootloader]\n# REBOOT REQUIRED\n")
		fmt.Fprintf(&b, "cmdline=+%s\n", strings.Join(cmdArgs(cmd), " "))
	}
	return b.String()
}

func cmdArgs(rs []advisor.Recommendation) []string {
	var args []string
	for _, r := range rs {
		if r.Proposed == "" {
			args = append(args, r.Param)
		} else {
			args = append(args, r.Param+"="+r.Proposed)
		}
	}
	return args
}

// Cmdline renders a GRUB_CMDLINE_LINUX addition. Empty when nothing applies.
func Cmdline(recs []advisor.Recommendation, m Meta) string {
	rs := pick(recs, "cmdline")
	if len(rs) == 0 {
		return ""
	}
	args := strings.Join(cmdArgs(rs), " ")
	var b strings.Builder
	b.WriteString(m.header("# "))
	b.WriteString("#\n# !!! REBOOT REQUIRED: this changes the kernel command line !!!\n#\n\n")
	for _, r := range rs {
		b.WriteString(comment(r, m))
	}
	fmt.Fprintf(&b, "\n# Add to /etc/default/grub:\nGRUB_CMDLINE_LINUX=\"$GRUB_CMDLINE_LINUX %s\"\n", args)
	b.WriteString("\n# Then regenerate the boot config and reboot:\n")
	b.WriteString("#   Debian/Ubuntu: update-grub\n")
	b.WriteString("#   RHEL/Fedora:   grubby --update-kernel=ALL --args=\"" + args + "\"\n")
	b.WriteString("#   or:            grub2-mkconfig -o /boot/grub2/grub.cfg\n")
	return b.String()
}

// NIC renders an ethtool shell script. Empty when nothing applies.
func NIC(recs []advisor.Recommendation, m Meta) string {
	rs := pick(recs, "nic")
	if len(rs) == 0 {
		return ""
	}
	type cmds struct{ rx, tx, combined string }
	by := map[string]*cmds{}
	var order []string
	var skipped []string
	for _, r := range rs {
		if !ifNameRe.MatchString(r.Target) {
			skipped = append(skipped, "# skipped interface "+strconv.QuoteToASCII(r.Target)+": not a valid interface name\n")
			continue
		}
		c := by[r.Target]
		if c == nil {
			c = &cmds{}
			by[r.Target] = c
			order = append(order, r.Target)
		}
		switch r.Param {
		case "rx_ring":
			c.rx = r.Proposed
		case "tx_ring":
			c.tx = r.Proposed
		case "combined":
			c.combined = r.Proposed
		}
	}
	sort.Strings(order)
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString(m.header("# "))
	b.WriteString("# These settings do not survive a reboot. Persist them with a systemd-networkd\n")
	b.WriteString("# .link file ([Link] RxBufferSize=, TxBufferSize=, CombinedChannels=), NetworkManager\n")
	b.WriteString("# (ethtool.ring-rx, ethtool.channels-combined) or a udev rule.\n\n")
	for _, r := range rs {
		if ifNameRe.MatchString(r.Target) {
			b.WriteString(comment(r, m))
		}
	}
	for _, sk := range dedupeStrings(skipped) {
		b.WriteString(sk)
	}
	b.WriteString("\n")
	for _, n := range order {
		c := by[n]
		if c.rx != "" || c.tx != "" {
			line := "ethtool -G " + shq(n)
			if c.rx != "" {
				line += " rx " + shq(c.rx)
			}
			if c.tx != "" {
				line += " tx " + shq(c.tx)
			}
			b.WriteString(line + "\n")
		}
		if c.combined != "" {
			fmt.Fprintf(&b, "ethtool -L %s combined %s\n", shq(n), shq(c.combined))
		}
	}
	return b.String()
}

// Runtime renders markdown advice for runtime and service results, grouped
// by app (the result's target).
func Runtime(recs []advisor.Recommendation, m Meta) string {
	rs := pick(recs, "runtime", "service")
	if len(rs) == 0 {
		return ""
	}
	groups := map[string][]advisor.Recommendation{}
	var names []string
	for _, r := range rs {
		g := r.Target
		if g == "" {
			g = "general"
		}
		if _, ok := groups[g]; !ok {
			names = append(names, g)
		}
		groups[g] = append(groups[g], r)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString(m.header("> "))
	b.WriteString("\n# Runtime advice\n")
	for _, n := range names {
		fmt.Fprintf(&b, "\n## %s\n", n)
		for _, r := range groups[n] {
			fmt.Fprintf(&b, "\n### %s\n\n", r.Title)
			advice := strings.TrimSpace(r.Advice)
			if advice == "" && r.Kind == "service" {
				advice = fmt.Sprintf("Service `%s` is %s; recommended: %s.", r.Param, r.Current, r.Proposed)
			}
			b.WriteString(advice + "\n")
			if r.Rationale != "" {
				fmt.Fprintf(&b, "\nWhy: %s\n", strings.Join(strings.Fields(r.Rationale), " "))
			}
			if r.Verify != "" {
				fmt.Fprintf(&b, "\nVerify: %s\n", r.Verify)
			}
			if cs := m.Cites.For(r); len(cs) > 0 {
				b.WriteString("\nSources:\n")
				for _, c := range cs {
					line := "- " + c.URL
					if c.Section != "" {
						line += " — " + c.Section
					}
					b.WriteString(line + "\n")
					if c.Quote != "" {
						fmt.Fprintf(&b, "  > \"%s\"\n", strings.Join(strings.Fields(c.Quote), " "))
					}
				}
			}
		}
	}
	return b.String()
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

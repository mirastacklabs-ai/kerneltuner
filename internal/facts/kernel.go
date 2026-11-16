package facts

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var releaseRE = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?`)

func collectKernel(r Root, f *Facts) {
	k := &f.Kernel
	if rel, ok := f.readReq(filepath.Join(r.Proc, "sys/kernel/osrelease")); ok {
		k.Release = rel
		if m := releaseRE.FindStringSubmatch(rel); m != nil {
			k.Major, _ = strconv.Atoi(m[1])
			k.Minor, _ = strconv.Atoi(m[2])
			k.Patch, _ = strconv.Atoi(m[3])
		} else {
			f.warnf("cannot parse kernel release %q", rel)
		}
	}
	if cl, ok := f.readReq(filepath.Join(r.Proc, "cmdline")); ok {
		k.Cmdline = cl
		k.CmdlineArgs = parseCmdline(cl)
	}
	if cc := readOpt(filepath.Join(r.Proc, "sys/net/ipv4/tcp_available_congestion_control")); cc != "" {
		k.AvailableCongestion = strings.Fields(cc)
	}
}

// parseCmdline splits a kernel command line into key -> value; bare flags map
// to "". Double-quoted values may contain spaces.
func parseCmdline(s string) map[string]string {
	out := map[string]string{}
	var toks []string
	var cur strings.Builder
	inQuote := false
	for _, c := range s {
		switch {
		case c == '"':
			inQuote = !inQuote
		case (c == ' ' || c == '\t' || c == '\n') && !inQuote:
			if cur.Len() > 0 {
				toks = append(toks, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		toks = append(toks, cur.String())
	}
	for _, t := range toks {
		k, v, _ := strings.Cut(t, "=")
		out[k] = v
	}
	return out
}

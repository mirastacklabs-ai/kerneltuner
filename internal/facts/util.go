package facts

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// readStr returns the whitespace-trimmed contents of path.
func readStr(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// readOpt is readStr that swallows errors (optional files).
func readOpt(path string) string {
	s, _ := readStr(path)
	return s
}

// readReq reads a file that should exist; on failure it records a warning.
func (f *Facts) readReq(path string) (string, bool) {
	s, err := readStr(path)
	if err != nil {
		f.warnf("cannot read %s: %v", path, err)
		return "", false
	}
	return s, true
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func atoiDef(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

func atou(s string) uint64 {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// linkBase returns the basename of the target of symlink path, or "".
func linkBase(path string) string {
	t, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return filepath.Base(t)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// bracketed returns the [active] token of a sysfs selector such as
// "always [madvise] never", plus all options without brackets.
func bracketed(s string) (active string, all []string) {
	for _, tok := range strings.Fields(s) {
		if strings.HasPrefix(tok, "[") && strings.HasSuffix(tok, "]") {
			tok = tok[1 : len(tok)-1]
			active = tok
		}
		all = append(all, tok)
	}
	return active, all
}

// parseCPUList parses "0-3,8,10-11".
func parseCPUList(s string) []int {
	var out []int
	for _, part := range strings.Split(strings.TrimSpace(s), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			continue
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil {
				continue
			}
		}
		for i := a; i <= b; i++ {
			out = append(out, i)
		}
	}
	return out
}

// numberedDirs lists entries of dir named prefix+<digits>, sorted numerically.
func numberedDirs(dir, prefix string) []int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var ids []int
	for _, e := range ents {
		rest, ok := strings.CutPrefix(e.Name(), prefix)
		if !ok || rest == "" {
			continue
		}
		if n, err := strconv.Atoi(rest); err == nil {
			ids = append(ids, n)
		}
	}
	sort.Ints(ids)
	return ids
}

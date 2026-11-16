package apply

import "strings"

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// valueMatches compares with whitespace normalisation. Selector-style files
// such as "[none] mq-deadline" match when the bracketed token equals want.
func valueMatches(got, want string) bool {
	if norm(got) == norm(want) {
		return true
	}
	return bracketed(got) != "" && bracketed(got) == norm(want)
}

func bracketed(s string) string {
	i := strings.IndexByte(s, '[')
	j := strings.IndexByte(s, ']')
	if i < 0 || j < i {
		return ""
	}
	return s[i+1 : j]
}

// restoreValue returns what to write back for a recorded raw value: the raw
// content, except for selector files where only the active token is writable.
func restoreValue(raw string) string {
	if b := bracketed(raw); b != "" {
		return b + "\n"
	}
	return raw
}

// merge produces the new content of a managed file, replacing lines for the
// keys being set and preserving everything else.
func merge(kind, existing string, isNew bool, ops []Op) []byte {
	sep, header := " = ", "# Managed by kerneltuner. Remove with: kerneltuner rollback <id>\n"
	if kind == fileDropin {
		sep, header = "=", "# Managed by kerneltuner.\n"
	}
	want := map[string]string{}
	var order []string
	for _, op := range ops {
		if _, ok := want[op.FileKey]; !ok {
			order = append(order, op.FileKey)
		}
		want[op.FileKey] = op.Proposed
	}
	var lines []string
	if existing != "" {
		lines = strings.Split(strings.TrimSuffix(existing, "\n"), "\n")
	}
	done := map[string]bool{}
	var out []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || t[0] == '#' || t[0] == ';' {
			out = append(out, ln)
			continue
		}
		k, _, ok := strings.Cut(t, "=")
		k = strings.TrimSpace(k)
		if ok {
			if v, hit := want[k]; hit {
				if !done[k] { // drop duplicates of a managed key
					out = append(out, k+sep+v)
					done[k] = true
				}
				continue
			}
		}
		out = append(out, ln)
	}
	var add []string
	for _, k := range order {
		if !done[k] {
			add = append(add, k+sep+want[k])
		}
	}
	if kind == fileDropin && len(add) > 0 {
		idx := -1
		for i, ln := range out {
			if strings.TrimSpace(ln) == "[Service]" {
				idx = i
			}
		}
		if idx < 0 {
			out = append(out, "[Service]")
			out = append(out, add...)
		} else {
			out = append(out[:idx+1], append(add, out[idx+1:]...)...)
		}
	} else {
		out = append(out, add...)
	}
	if isNew || existing == "" {
		out = append([]string{strings.TrimSuffix(header, "\n")}, out...)
	}
	return []byte(strings.Join(out, "\n") + "\n")
}

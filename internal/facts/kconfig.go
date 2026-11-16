package facts

import (
	"bytes"
	"compress/gzip"
	"io"
	"path/filepath"
	"strings"
)

const maxKconfig = 16 << 20 // decompressed bytes read from a kernel config

// collectKconfig fills Kernel.Config from <Proc>/config.gz, else from
// <root>/boot/config-<release> (root = parent of Proc). A missing config is
// not a warning: most kernels expose neither.
func collectKconfig(r Root, f *Facts) {
	var data []byte
	if b, err := readGzip(filepath.Join(r.Proc, "config.gz")); err == nil {
		data = b
	} else if rel := f.Kernel.Release; rel != "" && !strings.ContainsAny(rel, `/\`) && !strings.Contains(rel, "..") {
		// A release with a path separator or ".." must not pick another file.
		data, _ = ReadBounded(filepath.Join(filepath.Dir(r.Proc), "boot", "config-"+rel), maxKconfig)
	}
	if len(data) > 0 {
		f.Kernel.Config = ParseKconfig(string(data))
	}
}

func readGzip(path string) ([]byte, error) {
	b, err := ReadBounded(path, maxKconfig)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	return io.ReadAll(io.LimitReader(zr, maxKconfig))
}

// ParseKconfig parses a kernel .config: CONFIG_X=y -> "y", =m -> "m",
// =4096 -> "4096", ="str" -> "str" (unquoted), and "# CONFIG_X is not set"
// -> "n".
func ParseKconfig(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "CONFIG_"):
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
				v = v[1 : len(v)-1]
			}
			out[k] = v
		case strings.HasPrefix(line, "# CONFIG_") && strings.HasSuffix(line, " is not set"):
			out[strings.TrimSuffix(strings.TrimPrefix(line, "# "), " is not set")] = "n"
		}
	}
	return out
}

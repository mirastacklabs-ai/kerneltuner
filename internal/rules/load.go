package rules

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxRuleFile is the largest rule or sources file Load reads.
const maxRuleFile = 1 << 20

// rootAreas are the top-level directories that hold rule packs.
var rootAreas = []string{"kernel", "ai", "apps", "antipatterns"}

// Load reads all packs from fsys (embedded FS or --rules-dir overlay).
//
// Every *.yaml below kernel/, ai/, apps/ and antipatterns/ is a list of
// rules, except sources.yaml (a list of Source). ecosystem/tuned.yaml and
// ecosystem/bpftune.yaml are parsed into Ecosystem; either may be missing or
// empty. YAML is strict: unknown fields are an error.
func Load(fsys fs.FS) ([]Rule, Ecosystem, []Source, error) {
	var (
		rs   []Rule
		srcs []Source
	)
	for _, area := range rootAreas {
		if _, err := fs.Stat(fsys, area); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, Ecosystem{}, nil, err
		}
		err := fs.WalkDir(fsys, area, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || path.Ext(p) != ".yaml" {
				return nil
			}
			// A --rules-dir is untrusted input: only regular files up to
			// maxRuleFile bytes (no symlinks, devices, FIFOs).
			if !d.Type().IsRegular() {
				return nil
			}
			if info, err := d.Info(); err != nil || info.Size() > maxRuleFile {
				return nil
			}
			b, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			if path.Base(p) == "sources.yaml" {
				var s []Source
				if err := decodeStrict(b, &s); err != nil {
					return yamlErr(p, err)
				}
				for i := range s {
					s[i].Pack = path.Dir(p)
				}
				srcs = append(srcs, s...)
				return nil
			}
			var list []Rule
			if err := decodeStrict(b, &list); err != nil {
				return yamlErr(p, err)
			}
			for i := range list {
				list[i].Pack = path.Dir(p)
			}
			rs = append(rs, list...)
			return nil
		})
		if err != nil {
			return nil, Ecosystem{}, nil, err
		}
	}
	eco, err := loadEcosystem(fsys)
	if err != nil {
		return nil, Ecosystem{}, nil, err
	}
	return rs, eco, srcs, nil
}

// CheckPacks reports an error when fsys holds none of the pack areas (kernel/,
// ai/, apps/, antipatterns/). Load itself accepts such a tree (an empty rule
// set), which made `validate --rules-dir` on a wrong directory print "OK: 0
// rules"; validate calls this first. Typical causes are a missing directory
// and a copy made with `cp -r rules DEST`, which nests the packs in DEST/rules.
func CheckPacks(fsys fs.FS) error {
	for _, area := range rootAreas {
		if _, err := fs.Stat(fsys, area); err == nil {
			return nil
		}
	}
	hint := ""
	if _, err := fs.Stat(fsys, "rules"); err == nil {
		hint = `; it has a "rules" subdirectory, point at that instead`
	}
	return fmt.Errorf("no rule packs found: none of %s exists in the rules directory%s", strings.Join(rootAreas, "/, ")+"/", hint)
}

// yamlErr turns a yaml.v3 error into "file:line: message" lines (one per
// problem), so a broken sources.yaml or rule file is named with its line.
func yamlErr(file string, err error) error {
	var msgs []string
	var te *yaml.TypeError
	if errors.As(err, &te) {
		msgs = te.Errors
	} else {
		msgs = []string{strings.TrimPrefix(err.Error(), "yaml: ")}
	}
	var lines []string
	for _, m := range msgs {
		m = strings.TrimPrefix(strings.TrimSpace(m), "yaml: ")
		if rest, ok := strings.CutPrefix(m, "line "); ok {
			if n, msg, ok := strings.Cut(rest, ": "); ok {
				lines = append(lines, fmt.Sprintf("%s:%s: %s", file, n, msg))
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("%s: %s", file, m))
	}
	return errors.New(strings.Join(lines, "\n"))
}

// decodeStrict unmarshals YAML rejecting unknown fields. Empty input is fine.
func decodeStrict(b []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

type tunedFile struct {
	Profiles map[string]struct {
		Include string            `yaml:"include"`
		Sysctl  map[string]string `yaml:"sysctl"`
	} `yaml:"profiles"`
}

type bpftuneFile struct {
	Sysctls []string `yaml:"sysctls"`
}

func loadEcosystem(fsys fs.FS) (Ecosystem, error) {
	var eco Ecosystem
	if b, err := fs.ReadFile(fsys, "ecosystem/tuned.yaml"); err == nil {
		var tf tunedFile
		if err := decodeStrict(b, &tf); err != nil {
			return eco, fmt.Errorf("ecosystem/tuned.yaml: %w", err)
		}
		eco.Tuned = map[string]TunedProfile{}
		for name := range tf.Profiles {
			sysctl, err := resolveTuned(name, tf, nil)
			if err != nil {
				return eco, fmt.Errorf("ecosystem/tuned.yaml: %w", err)
			}
			eco.Tuned[name] = TunedProfile{Sysctl: sysctl}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return eco, err
	}
	if b, err := fs.ReadFile(fsys, "ecosystem/bpftune.yaml"); err == nil {
		var bf bpftuneFile
		if err := decodeStrict(b, &bf); err != nil {
			return eco, fmt.Errorf("ecosystem/bpftune.yaml: %w", err)
		}
		eco.Bpftune = append([]string(nil), bf.Sysctls...)
		sort.Strings(eco.Bpftune)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return eco, err
	}
	return eco, nil
}

// resolveTuned flattens a profile's include chain; the child wins. An
// include naming an unknown profile (a stock tuned profile we do not model)
// ends the chain.
func resolveTuned(name string, tf tunedFile, chain []string) (map[string]string, error) {
	for _, c := range chain {
		if c == name {
			return nil, fmt.Errorf("tuned profile include cycle: %v -> %s", chain, name)
		}
	}
	p, ok := tf.Profiles[name]
	if !ok {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	if p.Include != "" {
		parent, err := resolveTuned(p.Include, tf, append(chain, name))
		if err != nil {
			return nil, err
		}
		for k, v := range parent {
			out[k] = v
		}
	}
	for k, v := range p.Sysctl {
		out[k] = v
	}
	return out, nil
}

// Package fixture materializes testdata/fixtures/<name>/fixture.yaml into a fake root.
package fixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// Spec is a parsed fixture with `extends` already resolved.
type Spec struct {
	Description string             `yaml:"description"`
	Synthetic   bool               `yaml:"synthetic"`
	Extends     string             `yaml:"extends"`
	Files       map[string]string  `yaml:"files"`
	Symlinks    map[string]string  `yaml:"symlinks"`
	Values      map[string]float64 `yaml:"values"`
	Rates       map[string]float64 `yaml:"rates"`
}

// FixturesDir finds testdata/fixtures by walking up from the working
// directory until go.mod is found.
func FixturesDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "testdata", "fixtures"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("fixture: go.mod not found above " + wd)
		}
		dir = parent
	}
}

// Parse loads fixture name from the repo's fixtures dir with extends resolved.
func Parse(name string) (*Spec, error) {
	base, err := FixturesDir()
	if err != nil {
		return nil, err
	}
	return ParseIn(base, name)
}

// ParseIn is Parse with an explicit fixtures directory.
func ParseIn(base, name string) (*Spec, error) {
	return parse(base, name, nil)
}

func parse(base, name string, chain []string) (*Spec, error) {
	for _, n := range chain {
		if n == name {
			return nil, fmt.Errorf("fixture: extends cycle: %s -> %s", strings.Join(chain, " -> "), name)
		}
	}
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return nil, fmt.Errorf("fixture: invalid name %q", name)
	}
	b, err := os.ReadFile(filepath.Join(base, name, "fixture.yaml"))
	if err != nil {
		return nil, fmt.Errorf("fixture %q: %w", name, err)
	}
	var s Spec
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("fixture %q: %w", name, err)
	}
	if s.Extends == "" {
		return &s, nil
	}
	parent, err := parse(base, s.Extends, append(chain, name))
	if err != nil {
		return nil, err
	}
	merge(&parent.Files, s.Files)
	merge(&parent.Symlinks, s.Symlinks)
	merge(&parent.Values, s.Values)
	merge(&parent.Rates, s.Rates)
	parent.Description = s.Description
	parent.Synthetic = s.Synthetic
	parent.Extends = s.Extends
	return parent, nil
}

func merge[V any](dst *map[string]V, src map[string]V) {
	if *dst == nil {
		*dst = map[string]V{}
	}
	for k, v := range src {
		(*dst)[k] = v
	}
}

func safeJoin(dir, rel string) (string, error) {
	p := filepath.Join(dir, rel)
	if p != dir && !strings.HasPrefix(p, dir+string(filepath.Separator)) {
		return "", fmt.Errorf("fixture: path %q escapes root", rel)
	}
	return p, nil
}

// Materialize writes fixture name into dir and returns the Root plus the
// injected metric values and rates.
func Materialize(dir, name string) (facts.Root, map[string]float64, map[string]float64, error) {
	s, err := Parse(name)
	if err != nil {
		return facts.Root{}, nil, nil, err
	}
	return MaterializeSpec(dir, s)
}

// MaterializeSpec writes an already parsed Spec into dir.
func MaterializeSpec(dir string, s *Spec) (facts.Root, map[string]float64, map[string]float64, error) {
	for _, sub := range []string{"proc", "sys", "etc", "run", "var"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return facts.Root{}, nil, nil, err
		}
	}
	for rel, content := range s.Files {
		p, err := safeJoin(dir, rel)
		if err != nil {
			return facts.Root{}, nil, nil, err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return facts.Root{}, nil, nil, err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return facts.Root{}, nil, nil, err
		}
	}
	for rel, target := range s.Symlinks {
		p, err := safeJoin(dir, rel)
		if err != nil {
			return facts.Root{}, nil, nil, err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return facts.Root{}, nil, nil, err
		}
		if err := os.Symlink(target, p); err != nil {
			return facts.Root{}, nil, nil, err
		}
	}
	values, rates := s.Values, s.Rates
	if values == nil {
		values = map[string]float64{}
	}
	if rates == nil {
		rates = map[string]float64{}
	}
	return facts.RootAt(dir), values, rates, nil
}

// Load materializes fixture name into t.TempDir(). It fails the test on error.
func Load(t testing.TB, name string) (facts.Root, map[string]float64, map[string]float64) {
	t.Helper()
	root, values, rates, err := Materialize(t.TempDir(), name)
	if err != nil {
		t.Fatalf("fixture.Load(%q): %v", name, err)
	}
	return root, values, rates
}

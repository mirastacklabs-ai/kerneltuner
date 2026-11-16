// Package fixture materializes testdata/fixtures/<name>/fixture.yaml into a fake root.
package fixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

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

// EnvDir names the environment variable that points at the fixtures directory.
const EnvDir = "KERNELTUNER_FIXTURES"

var (
	explicitDir string // set by --fixtures-dir
	defaultDir  string // set by the fixturetest package, in tests only
)

// SetDir sets the fixtures directory (the hidden --fixtures-dir flag).
func SetDir(dir string) { explicitDir = dir }

// SetDefaultDir sets the directory used when neither SetDir nor the
// environment names one. Only the fixturetest package calls it.
func SetDefaultDir(dir string) { defaultDir = dir }

// FixturesDir returns the fixtures directory: the --fixtures-dir flag, else
// $KERNELTUNER_FIXTURES, else (tests only) the module's testdata/fixtures.
// It never searches the file system upwards from the working directory.
func FixturesDir() (string, error) {
	switch {
	case explicitDir != "":
		return explicitDir, nil
	case os.Getenv(EnvDir) != "":
		return os.Getenv(EnvDir), nil
	case defaultDir != "":
		return defaultDir, nil
	}
	return "", errors.New("fixture: no fixtures directory: pass --fixtures-dir DIR or set " + EnvDir)
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

// checkRel validates a fixture-relative path: relative, inside the root.
func checkRel(rel string) (string, error) {
	c := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(c) || c == "." || c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("fixture: path %q escapes root", rel)
	}
	return c, nil
}

// checkLink rejects absolute symlink targets and targets that leave the
// fixture root once resolved relative to the link's directory.
func checkLink(rel, target string) error {
	if filepath.IsAbs(target) || strings.HasPrefix(target, "/") {
		return fmt.Errorf("fixture: symlink %q has absolute target %q", rel, target)
	}
	res := filepath.Join(filepath.Dir(rel), target)
	if res == ".." || strings.HasPrefix(res, ".."+string(filepath.Separator)) {
		return fmt.Errorf("fixture: symlink %q target %q escapes the fixture root", rel, target)
	}
	return nil
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

// MaterializeSpec writes an already parsed Spec into dir. Every entry is
// created through an os.Root, so no component can resolve outside dir, and
// files are opened with O_NOFOLLOW.
func MaterializeSpec(dir string, s *Spec) (facts.Root, map[string]float64, map[string]float64, error) {
	fail := func(err error) (facts.Root, map[string]float64, map[string]float64, error) {
		return facts.Root{}, nil, nil, err
	}
	// Validate everything before touching the disk.
	files := map[string]string{}
	for rel, content := range s.Files {
		c, err := checkRel(rel)
		if err != nil {
			return fail(err)
		}
		files[c] = content
	}
	links := map[string]string{}
	for rel, target := range s.Symlinks {
		c, err := checkRel(rel)
		if err != nil {
			return fail(err)
		}
		if err := checkLink(c, target); err != nil {
			return fail(err)
		}
		links[c] = target
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = root.Close() }()
	for _, sub := range []string{"proc", "sys", "etc", "run", "var"} {
		if err := root.MkdirAll(sub, 0o755); err != nil {
			return fail(err)
		}
	}
	for rel, content := range files {
		if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return fail(err)
		}
		f, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
		if err != nil {
			return fail(err)
		}
		if _, err := f.WriteString(content); err != nil {
			_ = f.Close()
			return fail(err)
		}
		if err := f.Close(); err != nil {
			return fail(err)
		}
	}
	for rel, target := range links {
		if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return fail(err)
		}
		if err := root.Symlink(target, rel); err != nil {
			return fail(err)
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

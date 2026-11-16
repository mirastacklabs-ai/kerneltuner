// Package fixturetest loads fixtures for tests. It lives apart from package
// fixture so the production binary does not import "testing". Importing it
// (even with a blank import) points fixture.FixturesDir at the module's
// testdata/fixtures, located from this source file, never from the cwd.
package fixturetest

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
)

func init() {
	if _, file, _, ok := runtime.Caller(0); ok {
		// file = <module>/internal/fixturetest/fixturetest.go
		fixture.SetDefaultDir(filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixtures"))
	}
}

// Load materializes fixture name into t.TempDir(). It fails the test on error.
func Load(t testing.TB, name string) (facts.Root, map[string]float64, map[string]float64) {
	t.Helper()
	root, values, rates, err := fixture.Materialize(t.TempDir(), name)
	if err != nil {
		t.Fatalf("fixturetest.Load(%q): %v", name, err)
	}
	return root, values, rates
}

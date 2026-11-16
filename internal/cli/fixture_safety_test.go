package cli

import (
	"bytes"
	"strings"
	"testing"

	_ "github.com/mirastacklabs-ai/kerneltuner/internal/fixturetest"
)

func init() {
	// The tests may run as root in a container; --fixture refuses root.
	fixtureEUID = func() int { return 1000 }
}

func runCLI(args ...string) (string, error) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// R-Q02: --fixture is refused for root and for a real apply.
func TestFixtureRefusedAsRoot(t *testing.T) {
	old := fixtureEUID
	t.Cleanup(func() { fixtureEUID = old })
	fixtureEUID = func() int { return 0 }
	if _, err := runCLI("scan", "--fixture", "web-16c-64g-nginx"); err == nil || !strings.Contains(err.Error(), "refusing to run as root") {
		t.Fatalf("err = %v", err)
	}
}

func TestFixtureApplyRequiresDryRun(t *testing.T) {
	if _, err := runCLI("apply", "--fixture", "web-16c-64g-nginx", "--yes"); err == nil || !strings.Contains(err.Error(), "--dry-run") {
		t.Fatalf("err = %v", err)
	}
	if _, err := runCLI("apply", "--fixture", "web-16c-64g-nginx", "--dry-run"); err != nil {
		t.Fatalf("dry run: %v", err)
	}
}

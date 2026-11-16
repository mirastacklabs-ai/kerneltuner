package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes the CLI against a fake root holding somaxconn=128 and the seed
// rules in testdata/rules.
func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, "proc/sys/net/core/somaxconn")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("128\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return runIn(t, root, "testdata/rules", args...)
}

func runIn(t *testing.T, root, rulesDir string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"--root", root, "--rules-dir", rulesDir}, args...))
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func TestScanText(t *testing.T) {
	out, _, err := run(t, "scan")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"test.somaxconn", "128 -> 8192", "recommend", "Warnings:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestScanJSONAndAll(t *testing.T) {
	out, _, err := run(t, "--json", "scan", "--all")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	recs, _ := raw["recommendations"].([]any)
	if len(recs) < 2 {
		t.Errorf("recs = %v", recs)
	}
	found := false
	for _, r := range recs {
		m, _ := r.(map[string]any)
		if m["rule_id"] == "test.somaxconn" {
			found = true
			if m["status"] != "recommend" || m["proposed"] != "8192" || m["current"] != "128" {
				t.Errorf("somaxconn = %v", m)
			}
		}
	}
	if !found {
		t.Errorf("somaxconn missing: %v", recs)
	}
}

func TestScanFailOn(t *testing.T) {
	_, _, err := run(t, "scan", "--fail-on", "medium")
	if ExitCode(err) != 2 {
		t.Errorf("--fail-on medium: err=%v code=%d", err, ExitCode(err))
	}
	_, _, err = run(t, "scan", "--fail-on", "high")
	if err != nil {
		t.Errorf("--fail-on high: %v", err)
	}
	if _, _, err = run(t, "scan", "--fail-on", "bogus"); err == nil || ExitCode(err) != 1 {
		t.Errorf("bad level: %v", err)
	}
}

func TestScanDomainFilter(t *testing.T) {
	out, _, err := run(t, "--domain", "memory", "scan")
	if err != nil || strings.Contains(out, "test.somaxconn") {
		t.Errorf("filter ignored: %v\n%s", err, out)
	}
}

func TestExplain(t *testing.T) {
	out, _, err := run(t, "explain", "net.core.somaxconn")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"test.somaxconn", "Small backlog", "https://docs.kernel.org", "On this host", `current="128"`, `proposed="8192"`} {
		if !strings.Contains(out, want) {
			t.Errorf("explain lacks %q:\n%s", want, out)
		}
	}
	if _, _, err := run(t, "explain", "nope"); err == nil {
		t.Error("unknown rule accepted")
	}
	if out, _, err := run(t, "--json", "explain", "test.somaxconn"); err != nil || !json.Valid([]byte(out)) {
		t.Errorf("json: %v %s", err, out)
	}
}

func TestGenerateStdoutAndOut(t *testing.T) {
	out, _, err := run(t, "generate", "--format", "sysctl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "net.core.somaxconn = 8192") || !strings.Contains(out, "# test.somaxconn: Listen backlog cap") {
		t.Errorf("sysctl output:\n%s", out)
	}
	dir := t.TempDir()
	if _, _, err := run(t, "generate", "--format", "all", "--out", dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"90-kerneltuner.conf", "runtime.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if _, _, err := run(t, "generate", "--format", "bogus"); err == nil {
		t.Error("bad format accepted")
	}
}

func TestSources(t *testing.T) {
	out, _, err := run(t, "sources")
	if err != nil || !strings.Contains(out, "docs.kernel.org") || strings.Contains(out, "example.com/unused") {
		t.Errorf("active sources: %v\n%s", err, out)
	}
	out, _, err = run(t, "sources", "--all")
	if err != nil || !strings.Contains(out, "example.com/unused") {
		t.Errorf("all sources: %v\n%s", err, out)
	}
}

func TestValidate(t *testing.T) {
	out, _, err := run(t, "validate")
	if err != nil || !strings.Contains(out, "OK: 2 rules") {
		t.Fatalf("validate: %v\n%s", err, out)
	}
	// A broken pack fails with a non-zero exit and every problem listed.
	bad := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bad, "kernel/x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "kernel/x/r.yaml"), []byte("- id: Bad\n  kind: sysctl\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err = runIn(t, t.TempDir(), bad, "validate")
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 1 || strings.Count(out, "invalid:") < 3 {
		t.Errorf("bad pack: err=%v out=%s", err, out)
	}
}

func TestEmbeddedRulesValidate(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"validate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

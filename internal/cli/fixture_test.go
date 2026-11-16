package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestScanFixtureFlag(t *testing.T) {
	cmd := NewRootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"--rules-dir", "testdata/rules", "--json", "scan", "--all", "--fixture", "engine-facts"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v (%s)", err, errb.String())
	}
	var rep struct {
		Kernel   string   `json:"kernel"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if rep.Kernel != "6.8.0-45-generic" {
		t.Errorf("kernel = %q", rep.Kernel)
	}
	for _, w := range rep.Warnings {
		if strings.Contains(w, "PCI") {
			t.Errorf("noise warning %q", w)
		}
	}
}

func TestFixtureFlagHiddenAndRejectsUnknown(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"scan", "--help"})
	_ = cmd.Execute()
	if strings.Contains(out.String(), "--fixture") {
		t.Errorf("--fixture should be hidden:\n%s", out.String())
	}
	cmd = NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--rules-dir", "testdata/rules", "scan", "--fixture", "no-such-fixture"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--fixture") {
		t.Errorf("err = %v", err)
	}
}

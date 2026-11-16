package cli

import (
	"bytes"
	"errors"
	"testing"
)

func TestRootRegistersSubcommands(t *testing.T) {
	root := NewRootCmd()
	for _, name := range []string{"scan", "explain", "generate", "sources", "agent", "status", "apply", "rollback", "capture", "version"} {
		if c, _, err := root.Find([]string{name}); err != nil || c.Name() != name {
			t.Errorf("subcommand %q not registered", name)
		}
	}
}

func TestStubFails(t *testing.T) {
	root := NewRootCmd()
	root.SetArgs([]string{"scan"})
	if err := root.Execute(); !errors.Is(err, errNotImplemented) {
		t.Fatalf("got %v, want errNotImplemented", err)
	}
}

func TestVersionOutput(t *testing.T) {
	root := NewRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Version:")) {
		t.Errorf("unexpected output %q", buf.String())
	}
}

package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func hostileRecs() []advisor.Recommendation {
	return []advisor.Recommendation{{
		RuleID: "r.x\x1b[2J", Kind: "limit", Domain: "limits", Param: "LimitNOFILE",
		Target: "evil\x1b]0;pwn\x07\n| FAKE | ROW |", Current: "1\r2", Proposed: "9\x9b", Title: "t\x1b",
		Status: advisor.StatusRecommend, Risk: "low", Rationale: "why\x1b[31m red",
	}}
}

func noControls(t *testing.T, name, s string) {
	t.Helper()
	for _, r := range s {
		if r != '\n' && (r < 0x20 || (r >= 0x7f && r <= 0x9f)) {
			t.Errorf("%s: control character %q in output:\n%q", name, r, s)
			return
		}
	}
}

// R-Q04: no terminal escapes in text or Markdown reports.
func TestReportsStripControlCharacters(t *testing.T) {
	f := &facts.Facts{Warnings: []string{"warn\x1b[1m"}}
	f.Kernel.Release = "6.1\x1b[0m"
	m := Meta{Facts: f}
	var text, md bytes.Buffer
	if err := Text(&text, hostileRecs(), m, true); err != nil {
		t.Fatal(err)
	}
	if err := Markdown(&md, hostileRecs(), m, true); err != nil {
		t.Fatal(err)
	}
	noControls(t, "text", text.String())
	noControls(t, "markdown", md.String())
	// the newline in Target must not start a fake table row
	for _, ln := range strings.Split(md.String(), "\n") {
		if strings.HasPrefix(ln, "| FAKE") {
			t.Errorf("injected Markdown row: %q", ln)
		}
	}
}

// R-Q07: untrusted host text is escaped in Markdown.
func TestMarkdownEscapesHostFields(t *testing.T) {
	f := &facts.Facts{Warnings: []string{"[click](http://evil) <img src=x>"}}
	f.Kernel.Release = "6.1 [x](http://evil) *b*"
	f.CPU.Model = "<script>alert(1)</script> | # h"
	var md bytes.Buffer
	if err := Markdown(&md, nil, Meta{Facts: f}, true); err != nil {
		t.Fatal(err)
	}
	out := md.String()
	for _, raw := range []string{"[click](", "[x](http", "*b*", " <img", " <script>"} {
		if strings.Contains(out, raw) {
			t.Errorf("unescaped %q in:\n%s", raw, out)
		}
	}
	if !strings.Contains(out, `\[click\]`) || !strings.Contains(out, `\<script\>`) {
		t.Errorf("escapes missing:\n%s", out)
	}
	// a Target is a code span, not markup
	rs := hostileRecs()
	rs[0].Target = "[a](http://evil)"
	md.Reset()
	if err := Markdown(&md, rs, Meta{}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md.String(), "`[a](http://evil)`") {
		t.Errorf("target not in a code span:\n%s", md.String())
	}
}

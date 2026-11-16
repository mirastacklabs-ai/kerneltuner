package rules

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSourceRefAcceptsStringAndObject(t *testing.T) {
	fsys := fstest.MapFS{
		"kernel/x/a.yaml": {Data: []byte(`
- id: a
  sources:
    - https://one
    - {url: https://two, quote: "exact words", section: Heading}
    - url: https://three
`)},
		"kernel/x/sources.yaml": {Data: []byte("- url: https://one\n  title: One\n")},
	}
	rs, _, srcs, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	want := []SourceRef{{URL: "https://one"}, {URL: "https://two", Quote: "exact words", Section: "Heading"}, {URL: "https://three"}}
	if len(rs) != 1 || len(rs[0].Sources) != 3 {
		t.Fatalf("rules = %+v", rs)
	}
	for i, w := range want {
		if rs[0].Sources[i] != w {
			t.Errorf("source %d = %+v, want %+v", i, rs[0].Sources[i], w)
		}
	}
	if got := rs[0].SourceURLs(); strings.Join(got, ",") != "https://one,https://two,https://three" {
		t.Errorf("SourceURLs = %v", got)
	}
	if srcs[0].Title != "One" || srcs[0].Pack != "kernel/x" {
		t.Errorf("registry = %+v", srcs[0])
	}
}

func TestSourceRefRejectsUnknownKeysAndShapes(t *testing.T) {
	for name, y := range map[string]string{
		"unknown key": "- id: a\n  sources:\n    - {url: u, quotes: x}\n",
		"list":        "- id: a\n  sources:\n    - [u]\n",
	} {
		if _, _, _, err := Load(fstest.MapFS{"kernel/x/a.yaml": {Data: []byte(y)}}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestQuoteLimitAndCitationReport(t *testing.T) {
	good, _, srcs, err := Load(os.DirFS("testdata/packs"))
	if err != nil {
		t.Fatal(err)
	}
	r := good[0]
	r.Sources = []SourceRef{{URL: srcs[0].URL, Quote: strings.Repeat("x", MaxQuoteLen)}}
	if errs := ValidateWith([]Rule{r}, srcs, ""); len(errs) != 0 {
		t.Errorf("300-char quote rejected: %v", errs)
	}
	r.Sources = []SourceRef{{URL: srcs[0].URL, Quote: strings.Repeat("x", MaxQuoteLen+1)}}
	errs := ValidateWith([]Rule{r}, srcs, "")
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "limit is 300") {
		t.Errorf("301-char quote: %v", errs)
	}

	r.Sources = []SourceRef{{URL: srcs[0].URL, Quote: "q"}, {URL: srcs[0].URL}}
	rep := CheckCitations([]Rule{r, {ID: "ok", Sources: []SourceRef{{URL: "u", Quote: "q"}}}})
	if rep.Rules != 2 || rep.RulesWithoutQuotes != 1 || rep.Sources != 3 || rep.SourcesWithoutQuote != 1 || len(rep.Problems) != 1 {
		t.Errorf("report = %+v", rep)
	}
}

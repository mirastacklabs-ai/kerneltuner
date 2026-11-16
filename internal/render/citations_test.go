package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestTextFootnotesAndReferences(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, testRecs(), testMeta(), false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	// somaxconn cites [1] (src+quote) and [2] (src2); nic.ring shares [2].
	for _, want := range []string{"(> 0) [1,2]", "References\n[1] Linux kernel: IP sysctl — somaxconn (reviewed 2026-11-16)\n    \"somaxconn - INTEGER"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Count(out, "https://kernelnewbies.org/Linux_5.4") != 1 {
		t.Errorf("a source must be listed once:\n%s", out)
	}
	for _, l := range strings.Split(out[strings.Index(out, "References"):], "\n") {
		if len(l) > 100 {
			t.Errorf("reference line over 100 columns: %q", l)
		}
	}
	m := testMeta()
	m.NoCitations = true
	b.Reset()
	if err := Text(&b, testRecs(), m, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "References") || strings.Contains(b.String(), "[1]") {
		t.Errorf("--no-citations leaked citations:\n%s", b.String())
	}
}

func TestMarkdownGolden(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, append(dupRecs(), testRecs()...), testMeta(), false); err != nil {
		t.Fatal(err)
	}
	golden(t, "scan.md.golden", b.String())
	for _, want := range []string{"> **CONFLICT**", "| Status | Rule |", "## References", "<https://docs.kernel.org/networking/ip-sysctl.html>"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
}

func TestJSONSourcesAreObjects(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, testRecs(), testMeta(), false); err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Recommendations []struct {
			RuleID     string   `json:"rule_id"`
			Sources    []Cite   `json:"sources"`
			SourceURLs []string `json:"source_urls"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(b.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Recommendations {
		if r.RuleID != "net.core.somaxconn" {
			continue
		}
		if len(r.Sources) != 2 || r.Sources[0].Title != "Linux kernel: IP sysctl" || r.Sources[0].Project != "linux" ||
			r.Sources[0].Section != "somaxconn" || !strings.HasPrefix(r.Sources[0].Quote, "somaxconn - INTEGER") ||
			r.Sources[0].LastReviewed != "2026-11-16" || len(r.SourceURLs) != 1 || r.SourceURLs[0] != src {
			t.Errorf("sources = %+v urls = %v", r.Sources, r.SourceURLs)
		}
		return
	}
	t.Error("somaxconn missing from JSON")
}

func TestGenerateCommentsListEverySource(t *testing.T) {
	got := Sysctl(testRecs(), testMeta())
	for _, want := range []string{
		"# net.core.somaxconn: Listen backlog cap\n# source: " + src + " — somaxconn\n#   \"somaxconn - INTEGER",
		"# source: " + src2 + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testQuote = "somaxconn - INTEGER: Limit of socket listen() backlog."

func TestScanTextHasFootnotesAndReferences(t *testing.T) {
	out, _, err := run(t, "scan")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[2]\n", "References\n",
		"[2] Linux kernel: IP sysctl — somaxconn (reviewed 2026-01-01)",
		"\"" + testQuote + "\"", "https://docs.kernel.org/networking/ip-sysctl.html",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("scan lacks %q:\n%s", want, out)
		}
	}
	// the bare-URL rule shares the page but not the quote: its own reference
	if !strings.Contains(out, "[1] Linux kernel: IP sysctl (reviewed 2026-01-01)") {
		t.Errorf("no second reference:\n%s", out)
	}
	out, _, err = run(t, "scan", "--no-citations")
	if err != nil || strings.Contains(out, "\nReferences\n") || strings.Contains(out, "[1]") {
		t.Errorf("--no-citations: %v\n%s", err, out)
	}
}

func TestScanFormats(t *testing.T) {
	out, _, err := run(t, "scan", "--format", "md")
	if err != nil || !strings.HasPrefix(out, "# kerneltuner report") || !strings.Contains(out, "## References") || !strings.Contains(out, "> \""+testQuote) {
		t.Errorf("md: %v\n%s", err, out)
	}
	jsonOut, _, err := run(t, "scan", "--format", "json")
	if err != nil || !json.Valid([]byte(jsonOut)) {
		t.Fatalf("json: %v\n%s", err, jsonOut)
	}
	var rep struct {
		Recommendations []struct {
			RuleID  string `json:"rule_id"`
			Sources []struct {
				URL, Quote, Section, Project, Title string
				LastReviewed                        string `json:"last_reviewed"`
			} `json:"sources"`
			SourceURLs []string `json:"source_urls"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &rep); err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Recommendations {
		if r.RuleID != "test.somaxconn" {
			continue
		}
		s := r.Sources[0]
		if len(r.Sources) != 1 || s.Quote != testQuote || s.Section != "somaxconn" || s.Project != "linux" ||
			s.Title != "Linux kernel: IP sysctl" || s.LastReviewed != "2026-01-01" || len(r.SourceURLs) != 1 {
			t.Errorf("sources = %+v urls = %v", r.Sources, r.SourceURLs)
		}
	}
	if _, _, err := run(t, "scan", "--format", "xml"); err == nil {
		t.Error("bad format accepted")
	}
	if _, _, err := run(t, "--json", "scan", "--format", "md"); err == nil {
		t.Error("--json with --format md accepted")
	}
}

func TestExplainShowsEverySourceDetail(t *testing.T) {
	out, _, err := run(t, "explain", "test.somaxconn")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Sources:", "[1] Linux kernel: IP sysctl — somaxconn (reviewed 2026-01-01)", "\"" + testQuote + "\"", "https://docs.kernel.org/networking/ip-sysctl.html"} {
		if !strings.Contains(out, want) {
			t.Errorf("explain lacks %q:\n%s", want, out)
		}
	}
}

func TestGenerateCommentsCarryQuotes(t *testing.T) {
	out, _, err := run(t, "generate", "--format", "sysctl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# source: https://docs.kernel.org/networking/ip-sysctl.html — somaxconn\n#   \""+testQuote) {
		t.Errorf("generate lacks the quote:\n%s", out)
	}
}

func TestValidateStrictCitations(t *testing.T) {
	out, errOut, err := run(t, "validate")
	if err != nil || !strings.Contains(out, "OK: 2 rules") || !strings.Contains(errOut, "warning: 1 of 2 rules (1 of 2 sources) cite no verbatim quote") {
		t.Errorf("default: %v\nout=%s\nerr=%s", err, out, errOut)
	}
	out, _, err = run(t, "validate", "--strict-citations")
	if ExitCode(err) != 1 || !strings.Contains(out, "test.runtime: 1 of 1 source(s) have no quote") {
		t.Errorf("strict: %v\n%s", err, out)
	}
}

// sourcesPack writes a rule pack whose sources point at the test server.
func sourcesPack(t *testing.T, base string) string {
	t.Helper()
	dir := t.TempDir()
	pack := filepath.Join(dir, "kernel/network")
	if err := os.MkdirAll(pack, 0o755); err != nil {
		t.Fatal(err)
	}
	rule := `- id: x.good
  title: Good
  kind: runtime
  domain: network
  advice: a
  risk: low
  sources:
    - {url: BASE/good, quote: "backlog of pending connections"}
    - BASE/plain
  tests: [{fixture: minimal, expect: {status: recommend}}]
- id: x.bad
  title: Bad
  kind: runtime
  domain: network
  advice: a
  risk: low
  sources:
    - {url: BASE/good, quote: "words that are not there"}
    - {url: BASE/gone, quote: "q"}
  tests: [{fixture: minimal, expect: {status: recommend}}]
`
	srcs := `- {url: BASE/good, project: p, last_reviewed: "2026-01-01"}
- {url: BASE/plain, project: p, last_reviewed: "2026-01-01"}
- {url: BASE/gone, project: p, last_reviewed: "2026-01-01"}
`
	other := "- {url: BASE/other, project: p, last_reviewed: \"2026-01-01\"}\n"
	for name, body := range map[string]string{"kernel/network/r.yaml": rule, "kernel/network/sources.yaml": srcs, "apps/x/sources.yaml": other} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.ReplaceAll(body, "BASE", base)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSourcesCheck(t *testing.T) {
	var (
		mu  sync.Mutex
		uas []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		uas = append(uas, r.Header.Get("User-Agent"))
		mu.Unlock()
		if r.URL.Path == "/gone" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<p>The <b>backlog</b> of pending   connections.</p>"))
	}))
	defer srv.Close()
	dir := sourcesPack(t, srv.URL)

	out, _, err := runIn(t, t.TempDir(), dir, "sources", "--check")
	if ExitCode(err) != 1 {
		t.Fatalf("want exit 1, got %v\n%s", err, out)
	}
	for _, want := range []string{"QUOTE-MISSING " + srv.URL + "/good", "missing quote: \"words that are not there\"", "HTTP-ERROR    " + srv.URL + "/gone", "OK            " + srv.URL + "/plain", "4 source(s) checked"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, ua := range uas {
		if !strings.HasPrefix(ua, "kerneltuner-sources-check/") {
			t.Errorf("User-Agent = %q", ua)
		}
	}

	out, _, err = runIn(t, t.TempDir(), dir, "--json", "sources", "--check", "--only", "network")
	if ExitCode(err) != 1 {
		t.Fatalf("--only: %v", err)
	}
	var rs []struct{ URL, Status string }
	if err := json.Unmarshal([]byte(out), &rs); err != nil || len(rs) != 3 {
		t.Fatalf("json: %v %s", err, out)
	}
	if _, _, err := runIn(t, t.TempDir(), dir, "sources", "--check", "--only", "nope"); err == nil {
		t.Error("unknown pack accepted")
	}

	// A pack whose quotes all hold exits 0.
	good := t.TempDir()
	if err := os.MkdirAll(filepath.Join(good, "kernel/x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "kernel/x/sources.yaml"), []byte("- {url: "+srv.URL+"/ok, project: p}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runIn(t, t.TempDir(), good, "sources", "--check"); err != nil || !strings.Contains(out, "1 OK") {
		t.Errorf("good pack: %v\n%s", err, out)
	}
}

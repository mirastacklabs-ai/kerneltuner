// Package sourcecheck verifies that the registered upstream documents still
// exist and still contain the quotes rules cite from them. It is the only
// code in kerneltuner that talks to the network, and only when asked
// (`kerneltuner sources --check`). It sends nothing but a GET with a fixed
// User-Agent: no host data, no cookies.
package sourcecheck

import (
	"context"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Status is the verdict for one URL.
type Status string

// The possible verdicts.
const (
	OK           Status = "OK"
	QuoteMissing Status = "QUOTE-MISSING"
	Redirected   Status = "REDIRECTED"
	// SameSiteMoved is a warning, not a failure: the URL redirected to
	// another path on the same host (e.g. /latest/x to /v3.7/x) and every
	// quote was found on the final page.
	SameSiteMoved Status = "WARN-MOVED"
	HTTPError     Status = "HTTP-ERROR"
	Unreachable   Status = "UNREACHABLE"
)

// Defaults of Options.
const (
	DefaultTimeout     = 15 * time.Second
	DefaultConcurrency = 4
	maxBody            = 16 << 20
)

// Item is one registered URL with the quotes rules cite from it.
type Item struct {
	URL    string
	Pack   string
	Quotes []string
}

// Result is the outcome of checking one Item.
type Result struct {
	URL        string   `json:"url"`
	Pack       string   `json:"pack,omitempty"`
	Status     Status   `json:"status"`
	HTTPStatus int      `json:"http_status,omitempty"`
	FinalURL   string   `json:"final_url,omitempty"` // set when REDIRECTED or WARN-MOVED
	Missing    []string `json:"missing_quotes,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// Failed reports whether the result is a failure: anything but OK or a
// same-site move warning.
func (r Result) Failed() bool { return r.Status != OK && r.Status != SameSiteMoved }

// Options configures Check. The zero value gives the defaults.
type Options struct {
	Client      *http.Client  // default: a plain client with Timeout
	Timeout     time.Duration // per request; default 15s
	Concurrency int           // default 4
	UserAgent   string        // default "kerneltuner-sources-check/dev"
	// AllowHTTP permits http:// URLs and https-to-http redirects. For tests
	// against local servers only; the CLI never sets it.
	AllowHTTP bool
}

// MaxRedirects is the most redirects one check follows.
const MaxRedirects = 5

// redirectPolicy caps redirects and refuses an https to http downgrade.
func redirectPolicy(allowHTTP bool) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) > MaxRedirects {
			return fmt.Errorf("stopped after %d redirects", MaxRedirects)
		}
		if !allowHTTP && req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to non-https URL %s", req.URL)
		}
		return nil
	}
}

// Check fetches every item and classifies it. Results are in the order of
// items. Requests carry only a User-Agent header.
func Check(ctx context.Context, items []Item, o Options) []Result {
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Concurrency <= 0 {
		o.Concurrency = DefaultConcurrency
	}
	if o.UserAgent == "" {
		o.UserAgent = "kerneltuner-sources-check/dev"
	}
	client := &http.Client{}
	if o.Client != nil {
		c := *o.Client // copy: never mutate the caller's client
		client = &c
	}
	client.CheckRedirect = redirectPolicy(o.AllowHTTP)
	out := make([]Result, len(items))
	sem := make(chan struct{}, o.Concurrency)
	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = checkOne(ctx, client, items[i], o)
		}(i)
	}
	wg.Wait()
	return out
}

func checkOne(ctx context.Context, client *http.Client, it Item, o Options) Result {
	res := Result{URL: it.URL, Pack: it.Pack}
	if u, err := url.Parse(it.URL); err != nil || (u.Scheme != "https" && (!o.AllowHTTP || u.Scheme != "http")) {
		res.Status, res.Error = Unreachable, "only https URLs are checked"
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, it.URL, nil)
	if err != nil {
		res.Status, res.Error = Unreachable, err.Error()
		return res
	}
	req.Header.Set("User-Agent", o.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		res.Status, res.Error = Unreachable, err.Error()
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	res.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		res.Status, res.Error = HTTPError, resp.Status
		return res
	}
	moved := false
	if final := resp.Request.URL; movedAway(it.URL, final) {
		res.FinalURL = final.String()
		if !sameSite(it.URL, final) {
			res.Status = Redirected
			return res
		}
		moved = true
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		res.Status, res.Error = Unreachable, err.Error()
		return res
	}
	text, alt := string(body), ""
	if isHTML(resp.Header.Get("Content-Type"), body) {
		text = HTMLToText(text)
	} else {
		// A quote from a block comment spans lines that each start with " * ".
		alt = Normalize(stripCommentLeaders(text))
	}
	text = Normalize(text)
	for _, q := range it.Quotes {
		if nq := Normalize(q); nq != "" && !strings.Contains(text, nq) && (alt == "" || !strings.Contains(alt, nq)) {
			res.Missing = append(res.Missing, q)
		}
	}
	res.Status = OK
	switch {
	case len(res.Missing) > 0:
		res.Status = QuoteMissing
	case moved:
		res.Status = SameSiteMoved
	}
	return res
}

// isHTML reports whether a body is HTML: by Content-Type, or, when the type is
// absent or generic, because it starts like an HTML document. text/plain and
// every other type is compared raw (source files, man pages, rst, yaml), so
// text such as "<num>" or "#include <x>" is not mistaken for markup.
func isHTML(contentType string, body []byte) bool {
	mt, _, _ := mime.ParseMediaType(contentType)
	switch mt {
	case "text/html", "application/xhtml+xml":
		return true
	case "", "application/octet-stream":
		head := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 1024)])))
		return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
	}
	return false
}

// sameSite reports whether a redirect stayed on the same host (a leading
// "www." is ignored; a different port is a different site).
func sameSite(orig string, final *url.URL) bool {
	o, err := url.Parse(orig)
	if err != nil {
		return false
	}
	host := func(h string) string { return strings.TrimPrefix(strings.ToLower(h), "www.") }
	return host(o.Host) == host(final.Host)
}

// movedAway reports whether following redirects from orig ended on another
// host or path. A change of scheme, query or fragment, or of a trailing
// slash, is not a move.
func movedAway(orig string, final *url.URL) bool {
	o, err := url.Parse(orig)
	if err != nil {
		return false
	}
	trim := func(p string) string { return strings.TrimSuffix(p, "/") }
	return !strings.EqualFold(o.Host, final.Host) || trim(o.Path) != trim(final.Path)
}

var (
	reBlocks = regexp.MustCompile(`(?is)<(script|style|noscript|template)\b.*?</(script|style|noscript|template)\s*>`)
	reComm   = regexp.MustCompile(`(?s)<!--.*?-->`)
	reTags   = regexp.MustCompile(`(?s)<[^>]*>`)
)

// HTMLToText strips markup: it drops script, style and comments, removes
// tags and decodes entities. Tags are removed without inserting a space (so
// <code>tcp_rmem</code>[2] stays one word); Normalize makes the comparison
// insensitive to the whitespace this loses at block boundaries.
func HTMLToText(s string) string {
	s = reComm.ReplaceAllString(s, "")
	s = reBlocks.ReplaceAllString(s, "")
	s = reTags.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

var reLeader = regexp.MustCompile(`(?m)^[ \t]*\*+[ \t]?`)

// stripCommentLeaders drops the " * " that starts each line of a C block
// comment, so a quote spanning several comment lines can be matched in raw
// source files. It is only tried in addition to the plain comparison.
func stripCommentLeaders(s string) string { return reLeader.ReplaceAllString(s, "") }

var punct = strings.NewReplacer(
	"\u2018", "'", "\u2019", "'", "\u201c", "\"", "\u201d", "\"",
	"\u2013", "-", "\u2014", "-", "\u2212", "-", "\u2010", "-", "\u2011", "-",
	"\u00ad", "", "\u200b", "", "\u2026", "...",
)

// Normalize folds text for comparison: typographic quotes and dashes become
// ASCII, and all whitespace (including no-break spaces) is removed. Case is
// kept: quotes are verbatim.
func Normalize(s string) string {
	s = punct.Replace(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// Summary counts results per status, sorted by status name.
func Summary(rs []Result) string {
	n := map[Status]int{}
	for _, r := range rs {
		n[r.Status]++
	}
	var keys []string
	for s := range n {
		keys = append(keys, string(s))
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", n[Status(k)], k))
	}
	return strings.Join(parts, ", ")
}

package sourcecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const page = `<html><head><style>.x{}</style><script>var tcp_rmem = "nope";</script></head>
<body><h1>ip-sysctl</h1><p>The <code>tcp_rmem</code>[2] is the
maximum size&nbsp;of the&#32;receive   buffer &mdash; it is not capped.</p>
<p>Don&#39;t &ldquo;panic&rdquo;.</p></body></html>`

func server(t *testing.T) *httptest.Server {
	t.Helper()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(page)) }))
	t.Cleanup(other.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "kerneltuner-sources-check/test" {
			http.Error(w, "bad UA "+r.Header.Get("User-Agent"), http.StatusTeapot)
			return
		}
		if len(r.Header) > 3 || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			http.Error(w, "unexpected headers", http.StatusTeapot)
			return
		}
		_, _ = w.Write([]byte(page))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new/place", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new/place", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(page)) })
	mux.HandleFunc("/slash", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/slash/", http.StatusFound) })
	mux.HandleFunc("/slash/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(page)) })
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/slow", func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	const raw = "#include <linux/tcp.h>\n/* NOTE: sysctl <num> is set; use <code>x</code> */\n/*\n * NOTE: BBR might be used with the fq qdisc,\n * otherwise TCP falls back.\n */\n"
	mux.HandleFunc("/src.c", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(raw))
	})
	mux.HandleFunc("/untyped.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = nil // no Content-Type, no sniffing
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body><p>The <b>quick</b> fox</p></body></html>"))
	})
	mux.HandleFunc("/html-as-text", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("<p>The <b>quick</b> fox</p>"))
	})
	mux.HandleFunc("/latest/x", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v3.7/x", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/v3.7/x", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(page)) })
	mux.HandleFunc("/xhost", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/new/place", http.StatusMovedPermanently)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckClassifies(t *testing.T) {
	srv := server(t)
	items := []Item{
		{URL: srv.URL + "/ok", Pack: "kernel/network", Quotes: []string{
			"The tcp_rmem[2] is the maximum size of the receive buffer - it is not capped.",
			`Don't "panic".`,
		}},
		{URL: srv.URL + "/ok", Quotes: []string{"this sentence is not on the page", "it is not capped"}},
		{URL: srv.URL + "/ok"}, // no quotes: reachable is enough
		{URL: srv.URL + "/moved"},
		{URL: srv.URL + "/slash"},
		{URL: srv.URL + "/gone"},
		{URL: srv.URL + "/slow"},
		{URL: "http://127.0.0.1:1/closed"},
		{URL: srv.URL + "/ok#frag", Quotes: []string{"var tcp_rmem"}}, // script text is not page text
	}
	got := Check(context.Background(), items, Options{AllowHTTP: true, Timeout: 300 * time.Millisecond, UserAgent: "kerneltuner-sources-check/test"})
	want := []Status{OK, QuoteMissing, OK, SameSiteMoved, OK, HTTPError, Unreachable, Unreachable, QuoteMissing}
	for i, w := range want {
		if got[i].Status != w {
			t.Errorf("%s: status %s, want %s (%+v)", items[i].URL, got[i].Status, w, got[i])
		}
	}
	if got[0].Pack != "kernel/network" {
		t.Errorf("pack lost: %+v", got[0])
	}
	if len(got[1].Missing) != 1 || got[1].Missing[0] != "this sentence is not on the page" {
		t.Errorf("missing = %v", got[1].Missing)
	}
	if got[3].FinalURL != srv.URL+"/new/place" {
		t.Errorf("final url = %q", got[3].FinalURL)
	}
	if got[3].Failed() {
		t.Error("a same-site move must not fail")
	}
	if got[5].HTTPStatus != 404 || !got[5].Failed() || got[0].Failed() {
		t.Errorf("results = %+v / %+v", got[5], got[0])
	}
}

func TestRedirectPolicy(t *testing.T) {
	srv := server(t)
	quote := []string{"it is not capped"}
	got := Check(context.Background(), []Item{
		{URL: srv.URL + "/latest/x", Quotes: quote},                              // same host, new path: warning
		{URL: srv.URL + "/latest/x", Quotes: []string{"absent from final page"}}, // warning does not hide a missing quote
		{URL: srv.URL + "/xhost", Quotes: quote},                                 // other host: failure
	}, Options{AllowHTTP: true})
	if got[0].Status != SameSiteMoved || got[0].FinalURL != srv.URL+"/v3.7/x" || got[0].Failed() {
		t.Errorf("same-site: %+v", got[0])
	}
	if got[1].Status != QuoteMissing || !got[1].Failed() {
		t.Errorf("same-site, quote gone: %+v", got[1])
	}
	if got[2].Status != Redirected || !got[2].Failed() || got[2].FinalURL == "" {
		t.Errorf("cross-host: %+v", got[2])
	}
}

func TestContentTypeDecidesHTMLStripping(t *testing.T) {
	srv := server(t)
	got := Check(context.Background(), []Item{
		{URL: srv.URL + "/src.c", Quotes: []string{"#include <linux/tcp.h>", "sysctl <num> is set; use <code>x</code>", "NOTE: BBR might be used with the fq qdisc, otherwise TCP falls back."}},
		{URL: srv.URL + "/untyped.html", Quotes: []string{"The quick fox"}},
		{URL: srv.URL + "/html-as-text", Quotes: []string{"<p>The <b>quick</b> fox</p>"}},
		{URL: srv.URL + "/html-as-text", Quotes: []string{"The quick fox"}}, // text/plain is not stripped
		{URL: srv.URL + "/ok", Quotes: []string{"The tcp_rmem[2] is the maximum"}},
	}, Options{AllowHTTP: true, UserAgent: "kerneltuner-sources-check/test"})
	want := []Status{OK, OK, OK, QuoteMissing, OK}
	for i, w := range want {
		if got[i].Status != w {
			t.Errorf("#%d: %+v, want %s", i, got[i], w)
		}
	}
}

func TestCheckBoundsConcurrency(t *testing.T) {
	var cur, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		cur.Add(-1)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	var items []Item
	for i := 0; i < 16; i++ {
		items = append(items, Item{URL: srv.URL + "/p"})
	}
	for _, r := range Check(context.Background(), items, Options{AllowHTTP: true}) {
		if r.Status != OK {
			t.Fatalf("%+v", r)
		}
	}
	if p := peak.Load(); p > 4 || p < 2 {
		t.Errorf("peak concurrency = %d, want 2..4", p)
	}
}

func TestNormalizeAndHTMLToText(t *testing.T) {
	if got := Normalize(HTMLToText("<p>A&nbsp;b\n<b>c</b> &amp; d &#8212; e</p><!-- x -->")); got != "Abc&d-e" {
		t.Errorf("got %q", got)
	}
}

func TestHTTPSRequiredAndRedirectCap(t *testing.T) {
	srv := server(t)
	got := Check(context.Background(), []Item{{URL: srv.URL + "/ok"}}, Options{})
	if got[0].Status != Unreachable || !got[0].Failed() {
		t.Errorf("http URL accepted by default: %+v", got[0])
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/loop/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, r.URL.Path+"x", http.StatusFound) })
	loop := httptest.NewServer(mux)
	defer loop.Close()
	got = Check(context.Background(), []Item{{URL: loop.URL + "/loop/"}}, Options{AllowHTTP: true})
	if got[0].Status != Unreachable || !strings.Contains(got[0].Error, "redirects") {
		t.Errorf("redirect cap: %+v", got[0])
	}
	// https -> http downgrade is refused
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) }))
	defer plain.Close()
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, plain.URL, http.StatusFound) }))
	defer tls.Close()
	got = Check(context.Background(), []Item{{URL: tls.URL}}, Options{Client: tls.Client()})
	if got[0].Status != Unreachable || !strings.Contains(got[0].Error, "non-https") {
		t.Errorf("downgrade: %+v", got[0])
	}
}

package sourcecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	got := Check(context.Background(), items, Options{Timeout: 300 * time.Millisecond, UserAgent: "kerneltuner-sources-check/test"})
	want := []Status{OK, QuoteMissing, OK, Redirected, OK, HTTPError, Unreachable, Unreachable, QuoteMissing}
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
	if got[5].HTTPStatus != 404 || !got[5].Failed() || got[0].Failed() {
		t.Errorf("results = %+v / %+v", got[5], got[0])
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
	for _, r := range Check(context.Background(), items, Options{}) {
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

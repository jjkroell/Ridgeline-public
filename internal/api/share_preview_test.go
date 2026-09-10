package api

import (
	"bytes"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
	xhtml "golang.org/x/net/html"
)

const previewShell = `<!doctype html><html><head><meta charset="utf-8"><title>Generic</title><meta name="description" content="Generic"><meta property="og:title" content="Generic"><meta property="og:image" content="/icon.png"><meta name="twitter:card" content="summary"><script>const keep = "<title>inside script</title>";</script></head><body><div id="app">SPA</div><script src="/app.js"></script></body></html>`

func shareTestEnv(t *testing.T) (*store.Store, string, http.Handler, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	pkt, err := meshcore.DecodeHex(claimAdvertHex)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 10, 22, 6, 0, 0, time.UTC)
	if err = st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: at}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": previewShell, "share-site.json": `{"name":"Test Mesh","url":"https://mesh.example"}`, "about.html": "prerendered about", "app.js": "original JS"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", dir)
	return st, pkt.Advert.PublicKey, s.Handler(), dir
}
func previewRequest(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(method, path, nil))
	return rr
}
func headTags(t *testing.T, b string) map[string][]string {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if n.Data == "title" && n.FirstChild != nil {
				out["title"] = append(out["title"], n.FirstChild.Data)
			}
			if n.Data == "meta" {
				k := attrs["name"]
				if k == "" {
					k = attrs["property"]
				}
				out[k] = append(out[k], attrs["content"])
			}
			if n.Data == "link" && attrs["rel"] == "canonical" {
				out["canonical"] = append(out["canonical"], attrs["href"])
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}
func TestSharePreviewInitialHTMLAndPNG(t *testing.T) {
	_, key, h, _ := shareTestEnv(t)
	rr := previewRequest(h, "GET", "/nodes/"+strings.ToLower(key)+"?token=never-echo-me")
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	tags := headTags(t, rr.Body.String())
	for _, key := range []string{"title", "description", "canonical", "og:title", "og:description", "og:url", "og:image", "twitter:title", "twitter:description", "twitter:card", "twitter:image"} {
		if len(tags[key]) != 1 {
			t.Errorf("%s: %v", key, tags[key])
		}
	}
	if tags["canonical"][0] != "https://mesh.example/nodes/"+key {
		t.Fatal(tags["canonical"])
	}
	if strings.Contains(rr.Body.String(), "never-echo-me") {
		t.Fatal("query leaked")
	}
	if !strings.Contains(rr.Body.String(), `<script>const keep = "<title>inside script</title>";</script>`) || !strings.Contains(rr.Body.String(), `<body><div id="app">SPA</div><script src="/app.js"></script></body>`) {
		t.Fatal("app shell changed")
	}
	if tags["twitter:card"][0] != "summary_large_image" {
		t.Fatal(tags["twitter:card"])
	}
	imageURL, err := url.Parse(tags["og:image"][0])
	if err != nil {
		t.Fatal(err)
	}
	pngResp := previewRequest(h, "GET", imageURL.RequestURI())
	if pngResp.Code != 200 || pngResp.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("PNG: %d %s", pngResp.Code, pngResp.Body.String())
	}
	cfg, err := png.DecodeConfig(pngResp.Body)
	if err != nil || cfg.Width != 1200 || cfg.Height != 630 {
		t.Fatalf("PNG dimensions: %+v %v", cfg, err)
	}
	head := previewRequest(h, "HEAD", imageURL.RequestURI())
	if head.Code != 200 || head.Body.Len() != 0 {
		t.Fatal("HEAD response")
	}
	req := httptest.NewRequest("GET", imageURL.RequestURI(), nil)
	req.Header.Set("If-None-Match", pngResp.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	h.ServeHTTP(cached, req)
	if cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatalf("conditional response: %d", cached.Code)
	}
	// A hostile Host cannot change the configured canonical or image origin.
	req = httptest.NewRequest("GET", "https://attacker.example/nodes/"+key, nil)
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	cached = httptest.NewRecorder()
	h.ServeHTTP(cached, req)
	if strings.Contains(cached.Body.String(), "attacker.example") {
		t.Fatal("untrusted host in metadata")
	}
}
func TestSharePreviewFreshnessAndVisibility(t *testing.T) {
	st, key, h, _ := shareTestEnv(t)
	path := "/nodes/" + key
	before := headTags(t, previewRequest(h, "GET", path).Body.String())["og:image"][0]
	pkt, _ := meshcore.DecodeHex(claimAdvertHex)
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	after := headTags(t, previewRequest(h, "GET", path).Body.String())["og:image"][0]
	if before == after {
		t.Fatal("image revision didn't change with observation")
	}
	if err := st.RetireNode(key, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, "/share/card.png?path=" + url.QueryEscape(path)} {
		rr := previewRequest(h, "GET", p)
		if strings.HasPrefix(p, "/share/") && rr.Code != 404 {
			t.Fatal("retired image exposed")
		}
		if strings.Contains(rr.Body.String(), "Last advert observed") {
			t.Fatal("retired node data exposed")
		}
	}
	if err := st.UnretireNode(key); err != nil {
		t.Fatal(err)
	}
	if err := st.AddBlock(store.BlockNode, key, "Hidden", "test"); err != nil {
		t.Fatal(err)
	}
	if rr := previewRequest(h, "GET", "/share/card.png?path="+url.QueryEscape(path)); rr.Code != 404 {
		t.Fatal("quarantined image exposed")
	}
}
func TestSharePreviewFallbacksAndPages(t *testing.T) {
	st, _, h, _ := shareTestEnv(t)
	for p, want := range map[string]string{"/about": "prerendered about", "/app.js": "original JS", "/login?token=secret": previewShell, "/admin": previewShell, "/nodes/nope": previewShell, "/unknown": previewShell} {
		rr := previewRequest(h, "GET", p)
		if rr.Body.String() != want {
			t.Errorf("%s changed fallback", p)
		}
	}
	for path := range sharePages {
		for _, prefix := range []string{"", "/m"} {
			rr := previewRequest(h, "GET", prefix+path)
			if rr.Code != 200 || !strings.Contains(rr.Body.String(), "summary_large_image") {
				t.Errorf("%s: no server metadata", prefix+path)
			}
		}
	}
	for _, path := range []string{"/nodes/nope", "/nodes/" + strings.Repeat("A", 64), "/admin", "https://attacker.example"} {
		rr := previewRequest(h, "GET", "/share/card.png?path="+url.QueryEscape(path))
		if rr.Code != 404 {
			t.Errorf("%s: %d", path, rr.Code)
		}
	}
	if rr := previewRequest(h, "POST", "/share/card.png?path=/nodes"); rr.Code != 405 {
		t.Fatal("image accepted POST")
	}
	st.Close()
	rr := previewRequest(h, "GET", "/share/card.png?path=/nodes/"+strings.Repeat("A", 64))
	if rr.Code != 503 || rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("database failure cached or not handled")
	}
}
func TestShareEscapingAndCard(t *testing.T) {
	p := sharePreview{Title: `<script>alert("x")</script> & 'hello'`, Description: `"><img src=x onerror=alert(1)>`, Path: "/nodes"}
	b, err := shareHTML([]byte(previewShell), shareSite{Name: "A & B", URL: "https://mesh.example"}, p)
	if err != nil {
		t.Fatal(err)
	}
	tags := headTags(t, string(b))
	if tags["og:title"][0] != p.Title+" · A & B" {
		t.Fatal("not escaped roundtrip")
	}
	if bytes.Contains(b, []byte(`<img src=x`)) || bytes.Contains(b, []byte(`<script>alert`)) {
		t.Fatal("HTML injection")
	}
	if cleanShareText("hello\u202E\nworld", 20) != "hello world" {
		t.Fatal("unsafe control characters")
	}
	for _, title := range []string{strings.Repeat("Wide name ", 50), "松 🐟 Côte d’Azur", ""} {
		p.Title = title
		b, err := renderShareCard(shareSite{Name: strings.Repeat("Long site ", 20)}, p)
		if err != nil || len(b) == 0 {
			t.Fatalf("render %q: %v", title, err)
		}
	}
	// Optional visual fixture, with explicitly synthetic public data.
	if dest := os.Getenv("RIDGELINE_SHARE_PREVIEW_SAMPLE"); dest != "" {
		p = sharePreview{Title: "Cokley Ridge", Path: "/nodes/" + strings.Repeat("48", 32), Node: &store.ShareNode{PublicKey: strings.Repeat("48", 32), Name: "Cokley Ridge", Role: "repeater", FirstSeen: "2026-08-12T06:41:00Z", LastAdvert: "2026-09-10T22:06:00Z"}}
		b, err := renderShareCard(shareSite{Name: "Ridgeline", URL: "https://mesh.example"}, p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

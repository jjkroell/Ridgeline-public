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
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
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
	if !strings.Contains(tags["og:image:alt"][0], key) || !strings.Contains(tags["twitter:image:alt"][0], key) {
		t.Fatal("full public key missing from accessible image descriptions")
	}
	if rr.Header().Get("Cache-Control") != "public, max-age=300, must-revalidate" {
		t.Fatal("document must revalidate before long-lived image cache")
	}
	imageURL, err := url.Parse(tags["og:image"][0])
	if err != nil {
		t.Fatal(err)
	}
	pngResp := previewRequest(h, "GET", imageURL.RequestURI())
	if pngResp.Code != 200 || pngResp.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("PNG: %d %s", pngResp.Code, pngResp.Body.String())
	}
	if pngResp.Header().Get("Cache-Control") != "public, max-age=86400, must-revalidate" {
		t.Fatal("identity card should allow a one-day cache")
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
func TestSharePreviewStableIdentityAndVisibility(t *testing.T) {
	st, key, h, _ := shareTestEnv(t)
	path := "/nodes/" + key
	before := previewRequest(h, "GET", path)
	imageURL := headTags(t, before.Body.String())["og:image"][0]
	parsed, err := url.Parse(imageURL)
	if err != nil {
		t.Fatal(err)
	}
	beforePNG := previewRequest(h, "GET", parsed.RequestURI())
	pkt, _ := meshcore.DecodeHex(claimAdvertHex)
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	after := previewRequest(h, "GET", path)
	if !bytes.Equal(before.Body.Bytes(), after.Body.Bytes()) || before.Header().Get("ETag") != after.Header().Get("ETag") {
		t.Fatal("new observations must not invalidate identity metadata or its image revision")
	}
	afterPNG := previewRequest(h, "GET", parsed.RequestURI())
	if !bytes.Equal(beforePNG.Body.Bytes(), afterPNG.Body.Bytes()) || beforePNG.Header().Get("ETag") != afterPNG.Header().Get("ETag") {
		t.Fatal("new observations must not change card bytes or ETag")
	}
	// Names remain mutable: a real identity edit must invalidate the revision.
	pkt.Advert.Name = "Renamed Ridge"
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Date(2026, 9, 11, 2, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	renamed := previewRequest(h, "GET", path)
	if headTags(t, renamed.Body.String())["og:image"][0] == imageURL {
		t.Fatal("rename must change image revision")
	}
	if !strings.Contains(renamed.Body.String(), "Renamed Ridge") {
		t.Fatal("rename not reflected in metadata")
	}
	if bytes.Equal(beforePNG.Body.Bytes(), previewRequest(h, "GET", parsed.RequestURI()).Body.Bytes()) {
		t.Fatal("old image URL must still resolve current identity after rename")
	}
	if err := st.RetireNode(key, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, "/share/card.png?path=" + url.QueryEscape(path)} {
		rr := previewRequest(h, "GET", p)
		if strings.HasPrefix(p, "/share/") {
			if rr.Code != 404 {
				t.Fatal("retired image exposed")
			}
		} else if rr.Body.String() != previewShell {
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

func TestShareCardLayouts(t *testing.T) {
	_, key, h, _ := shareTestEnv(t)
	for name, layout := range shareCardLayouts {
		t.Run(name, func(t *testing.T) {
			mono, err := opentype.NewFace(cardMono, &opentype.FaceOptions{Size: float64(layout.KeySize), DPI: 72, Hinting: font.HintingFull})
			if err != nil {
				t.Fatal(err)
			}
			defer mono.Close()
			// At a 320px-wide thumbnail the complete key must stay at least 12px.
			if float64(layout.KeySize)*320/float64(layout.Width) < 12 {
				t.Fatal("public key too small at mobile preview width")
			}
			for _, key := range []string{strings.Repeat("F", 64), strings.Repeat("0123456789ABCDEF", 4)} {
				lines := shareKeyLines(key, layout.KeyDigits)
				if strings.ReplaceAll(strings.Join(lines, ""), " ", "") != key {
					t.Fatal("card lost or reordered public-key digits")
				}
				for i, row := range lines {
					bounds, advance := font.BoundString(mono, row)
					if advance.Ceil() > layout.Width-2*layout.Margin || bounds.Min.X.Floor()+layout.Margin < 0 || bounds.Max.X.Ceil()+layout.Margin > layout.Width-layout.Margin {
						t.Fatal("full key overflows horizontally")
					}
					baseline := layout.KeyY + i*layout.KeyGap
					if baseline+bounds.Min.Y.Floor() <= layout.KeyLabelY || baseline+bounds.Max.Y.Ceil() > layout.Height-24 {
						t.Fatal("full key clipped vertically or overlaps label")
					}
				}
			}
			f, err := opentype.NewFace(cardBold, &opentype.FaceOptions{Size: float64(layout.TitleSize), DPI: 72, Hinting: font.HintingFull})
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			for _, title := range []string{"Cokley Ridge", strings.Repeat("W", 100), strings.Repeat("A long name ", 10), "松 🐟 Côte d’Azur", ""} {
				for i, row := range shareCardLines(title, f, layout.TitleWidth, 2) {
					bounds, advance := font.BoundString(f, row)
					if advance.Ceil() > layout.TitleWidth || layout.TitleY+i*layout.TitleGap+bounds.Max.Y.Ceil() >= layout.RuleY {
						t.Fatalf("title overflows: %q", row)
					}
				}
			}
			for _, path := range []string{"/nodes/" + key, "/nodes"} {
				resp := previewRequest(h, "GET", "/share/card.png?path="+url.QueryEscape(path)+"&format="+name)
				cfg, err := png.DecodeConfig(resp.Body)
				if resp.Code != 200 || err != nil || cfg.Width != layout.Width || cfg.Height != layout.Height {
					t.Fatalf("%s PNG: %d, %+v, %v", name, resp.Code, cfg, err)
				}
			}
		})
	}
	plain := previewRequest(h, "GET", "/share/card.png?path=/nodes")
	explicit := previewRequest(h, "GET", "/share/card.png?path=/nodes&format=wide")
	if !bytes.Equal(plain.Body.Bytes(), explicit.Body.Bytes()) {
		t.Fatal("default must remain wide")
	}
	for _, format := range []string{"huge", "100000x100000", "../story", "WIDE"} {
		rr := previewRequest(h, "GET", "/share/card.png?path=/nodes&format="+url.QueryEscape(format))
		if rr.Code != 400 || rr.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unknown format must fail without caching")
		}
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
		b, err := renderShareCard(shareSite{Name: strings.Repeat("Long site ", 20)}, p, shareCardLayouts["wide"])
		if err != nil || len(b) == 0 {
			t.Fatalf("render %q: %v", title, err)
		}
	}
	// Optional visual fixture, with explicitly synthetic public data.
	if dest := os.Getenv("RIDGELINE_SHARE_PREVIEW_SAMPLE"); dest != "" {
		const exampleKey = "00693EEBEECE35F80217D5A26F2E7208BEC362E68B251FAC36CE7A7D9A33763B"
		p = sharePreview{Title: "Cokley Ridge", Path: "/nodes/" + exampleKey, Node: &store.ShareNode{PublicKey: exampleKey, Name: "Cokley Ridge", Role: "repeater"}}
		for name, layout := range shareCardLayouts {
			b, err := renderShareCard(shareSite{Name: "Ridgeline", URL: "https://mesh.example"}, p, layout)
			if err != nil {
				t.Fatal(err)
			}
			path := dest
			if name != "wide" {
				path = strings.TrimSuffix(dest, ".png") + "-" + name + ".png"
			}
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

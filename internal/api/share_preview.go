package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jjkroell/ridgeline/internal/store"
	xhtml "golang.org/x/net/html"
)

type shareSite struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
type sharePreview struct {
	Title, Description, Path string
	Node                     *store.ShareNode
}

// Public section routes use stable descriptions. Entity data is loaded only for
// node routes; account, admin, token-bearing and unknown paths keep the SPA shell.
var sharePages = map[string]sharePreview{
	"/":          {Title: "MeshCore Observatory", Description: "Explore the mesh: nodes, coverage and packet activity."},
	"/nodes":     {Title: "Nodes", Description: "Explore the MeshCore node and repeater directory."},
	"/live":      {Title: "Live packet feed", Description: "Follow packets heard by this MeshCore observatory."},
	"/channels":  {Title: "Channels", Description: "Explore public channel activity on the mesh."},
	"/map":       {Title: "Coverage map", Description: "Explore MeshCore nodes and observed coverage."},
	"/live-map":  {Title: "Live signal map", Description: "See packets and observed links across the mesh."},
	"/analytics": {Title: "Network analytics", Description: "Explore observed mesh activity and link quality."},
	"/topology":  {Title: "Mesh topology", Description: "Explore observed connections between MeshCore nodes."},
	"/identity":  {Title: "Hash-ID planner", Description: "Inspect MeshCore public-key prefix ambiguity at each path width."},
	"/observers": {Title: "Observers", Description: "Meet the receive-only stations listening to this mesh."},
}

func previewPath(path string) string {
	// /m section links share the same public destination as their desktop counterpart.
	if path == "/m" {
		return "/"
	}
	if strings.HasPrefix(path, "/m/") {
		path = strings.TrimPrefix(path, "/m")
	}
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func (s *Server) lookupPreview(r *http.Request, path string) (*sharePreview, error) {
	path = previewPath(path)
	if p, ok := sharePages[path]; ok {
		p.Path = path
		return &p, nil
	}
	if !strings.HasPrefix(path, "/nodes/") {
		return nil, nil
	}
	key := strings.TrimPrefix(path, "/nodes/")
	if len(key) != 64 {
		return nil, nil
	}
	if _, err := hex.DecodeString(key); err != nil {
		return nil, nil
	}
	key = strings.ToUpper(key)
	n, err := s.store.NodeSharePreview(r.Context(), key)
	if err != nil || n == nil {
		return nil, err
	}
	title := cleanShareText(n.Name, 100)
	if title == "" {
		title = "Node " + key[:12]
	}
	n.Name = title
	role := shareRole(n.Role)
	desc := fmt.Sprintf("%s on the MeshCore mesh. Explore this node’s activity, coverage and connections.", role)
	return &sharePreview{Title: title, Description: desc, Path: "/nodes/" + key, Node: n}, nil
}

func shareRole(role string) string {
	switch role {
	case "repeater":
		return "Repeater"
	case "companion":
		return "Companion"
	case "room", "room_server":
		return "Room server"
	default:
		return "MeshCore node"
	}
}
func cleanShareText(s string, limit int) string {
	var out strings.Builder
	for s != "" {
		cluster, emoji := nextShareCluster(s)
		s = s[len(cluster):]
		if emoji != nil {
			// Preserve joiners and tag characters only inside recognized emoji.
			out.WriteString(cluster)
		} else {
			r, _ := utf8.DecodeRuneInString(cluster)
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				r = ' '
			}
			out.WriteRune(r)
		}
	}
	clean := strings.Join(strings.Fields(out.String()), " ")
	if utf8.RuneCountInString(clean) <= limit {
		return clean
	}
	out.Reset()
	count := 0
	for clean != "" {
		cluster, _ := nextShareCluster(clean)
		n := utf8.RuneCountInString(cluster)
		if count+n >= limit {
			break
		}
		out.WriteString(cluster)
		count += n
		clean = clean[len(cluster):]
	}
	return strings.TrimSpace(out.String()) + "…"
}

func (p *sharePreview) revision(site shareSite) string {
	b, _ := json.Marshal(struct {
		Version int
		Site    shareSite
		Preview *sharePreview
	}{4, site, p})
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:12])
}
func (p *sharePreview) imageURL(site shareSite) string {
	return site.URL + "/share/card.png?path=" + url.QueryEscape(p.Path) + "&v=" + p.revision(site)
}

// shareHandler renders only the document head, in the existing Go process.
// Interactive Svelte routes remain client-rendered. No user-agent sniffing,
// external requests, browser renderer, new service or session access is needed.
func (s *Server) shareHandler(next http.HandlerFunc) http.HandlerFunc {
	shell, err := os.ReadFile(filepath.Join(s.webDir, "index.html"))
	if err != nil {
		return next
	}
	site := shareSite{Name: "Ridgeline"}
	if b, err := os.ReadFile(filepath.Join(s.webDir, "share-site.json")); err == nil {
		if err := json.Unmarshal(b, &site); err != nil {
			s.log.Warn("invalid share-site.json; previews disabled")
			return next
		}
	}
	site.Name = cleanShareText(site.Name, 60)
	if site.Name == "" {
		site.Name = "Ridgeline"
	}
	site.URL = strings.TrimRight(site.URL, "/")
	if site.URL != "" {
		u, err := url.Parse(site.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			s.log.Warn("share site URL must be an HTTP(S) origin; previews disabled")
			return next
		}
	}
	renderSlots := make(chan struct{}, 4) // Bound concurrent PNG memory/CPU; no unbounded cache.
	return func(w http.ResponseWriter, r *http.Request) {
		imageRequest := r.URL.Path == "/share/card.png"
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if imageRequest {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			next(w, r)
			return
		}
		path := r.URL.Path
		layout := shareCardLayouts["wide"]
		if imageRequest {
			path = r.URL.Query().Get("path")
			if format := r.URL.Query().Get("format"); format != "" {
				var ok bool
				layout, ok = shareCardLayouts[format]
				if !ok {
					w.Header().Set("Cache-Control", "no-store")
					http.Error(w, "Unknown card format", http.StatusBadRequest)
					return
				}
			}
		}
		p, err := s.lookupPreview(r, path)
		if err != nil {
			s.log.Error("load share preview", "error", err)
			w.Header().Set("Cache-Control", "no-store")
			if imageRequest {
				http.Error(w, "Preview temporarily unavailable", http.StatusServiceUnavailable)
			} else {
				next(w, r)
			}
			return
		}
		if p == nil {
			if imageRequest {
				w.Header().Set("Cache-Control", "no-store")
				http.NotFound(w, r)
			} else {
				next(w, r)
			}
			return
		}
		// Registered assets and prerendered files retain their original response.
		if !imageRequest && p.Node == nil && p.Path != "/" {
			if info, err := os.Stat(filepath.Join(s.webDir, strings.TrimPrefix(p.Path, "/")+".html")); err == nil && !info.IsDir() {
				next(w, r)
				return
			}
		}
		var body []byte
		contentType := "text/html; charset=utf-8"
		if imageRequest {
			select {
			case renderSlots <- struct{}{}:
				defer func() { <-renderSlots }()
			default:
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Retry-After", "1")
				http.Error(w, "Preview busy", http.StatusServiceUnavailable)
				return
			}
			body, err = renderShareCard(site, *p, layout)
			contentType = "image/png"
		} else {
			body, err = shareHTML(shell, site, *p)
		}
		if err != nil {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Preview unavailable", http.StatusServiceUnavailable)
			return
		}
		// Identity cards stay stable across new observations. Recheck HTML sooner
		// so renames get a new revision URL; cache PNGs for a day. These URLs are
		// not immutable: the endpoint still serves the current public identity.
		cacheControl := "public, max-age=300, must-revalidate"
		if imageRequest {
			cacheControl = "public, max-age=86400, must-revalidate"
		}
		w.Header().Set("Cache-Control", cacheControl)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		sum := sha256.Sum256(body)
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sum))
		http.ServeContent(w, r, "preview", time.Time{}, bytes.NewReader(body))
	}
}

// Tokenize rather than regex-rewriting HTML. Preserve scripts, Svelte markers,
// style tags and the entire body byte-for-byte; remove all competing head tags.
func shareHTML(shell []byte, site shareSite, p sharePreview) ([]byte, error) {
	z := xhtml.NewTokenizer(bytes.NewReader(shell))
	var out bytes.Buffer
	inHead, skipTitle, wrote := false, false, false
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			if z.Err() != io.EOF {
				return nil, z.Err()
			}
			break
		}
		raw := append([]byte(nil), z.Raw()...)
		if tt == xhtml.StartTagToken || tt == xhtml.EndTagToken || tt == xhtml.SelfClosingTagToken {
			tok := z.Token()
			if tok.Data == "head" {
				if tt == xhtml.StartTagToken {
					inHead = true
				}
				if tt == xhtml.EndTagToken {
					out.WriteString(shareHead(site, p))
					inHead = false
					wrote = true
				}
			}
			if inHead && tok.Data == "title" {
				skipTitle = tt != xhtml.EndTagToken
				continue
			}
			if inHead && (tok.Data == "meta" || tok.Data == "link") {
				remove := false
				for _, a := range tok.Attr {
					v := strings.ToLower(a.Val)
					if tok.Data == "meta" && (a.Key == "name" || a.Key == "property") && (v == "description" || strings.HasPrefix(v, "og:") || strings.HasPrefix(v, "twitter:")) {
						remove = true
					}
					if tok.Data == "link" && a.Key == "rel" {
						for _, rel := range strings.Fields(v) {
							if rel == "canonical" {
								remove = true
							}
						}
					}
				}
				if remove {
					continue
				}
			}
		}
		if !skipTitle {
			out.Write(raw)
		}
	}
	if !wrote {
		return nil, fmt.Errorf("missing document head")
	}
	return out.Bytes(), nil
}
func shareHead(site shareSite, p sharePreview) string {
	esc := html.EscapeString
	title := p.Title + " · " + site.Name
	image := p.imageURL(site)
	alt := p.Title + " — " + p.Description
	if p.Node != nil {
		alt += " Public key: " + p.Node.PublicKey + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<title data-ridgeline-share>%s</title>\n<link data-ridgeline-share rel=\"canonical\" href=\"%s\">\n", esc(title), esc(site.URL+p.Path))
	for _, tag := range [][3]string{
		{"name", "description", p.Description}, {"property", "og:type", "website"},
		{"property", "og:site_name", site.Name}, {"property", "og:title", title},
		{"property", "og:description", p.Description}, {"property", "og:url", site.URL + p.Path},
		{"property", "og:image", image}, {"property", "og:image:type", "image/png"},
		{"property", "og:image:width", "1200"}, {"property", "og:image:height", "630"},
		{"property", "og:image:alt", alt},
		{"name", "twitter:card", "summary_large_image"}, {"name", "twitter:title", title},
		{"name", "twitter:description", p.Description}, {"name", "twitter:image", image},
		{"name", "twitter:image:alt", alt},
	} {
		fmt.Fprintf(&b, "<meta data-ridgeline-share %s=\"%s\" content=\"%s\">\n", tag[0], tag[1], esc(tag[2]))
	}
	return b.String()
}

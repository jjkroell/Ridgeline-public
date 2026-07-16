package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// staticHandler serves the built SPA from dir, falling back to index.html for
// any path that doesn't resolve to a real file so client-side routing works.
func staticHandler(dir string) http.HandlerFunc {
	index := filepath.Join(dir, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		// Never let a non-API path escape the web dir.
		clean := filepath.Clean(r.URL.Path)
		if strings.Contains(clean, "..") {
			http.NotFound(w, r)
			return
		}

		p := filepath.Join(dir, clean)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
		// Prerendered pages (adapter-static) land as "<route>.html" — e.g. /about ->
		// about.html. Serve those before falling back so their real, crawlable HTML
		// is delivered instead of the empty SPA shell.
		if clean != "/" && !strings.HasSuffix(clean, ".html") {
			if info, err := os.Stat(p + ".html"); err == nil && !info.IsDir() {
				http.ServeFile(w, r, p+".html")
				return
			}
		}
		// SPA fallback.
		http.ServeFile(w, r, index)
	}
}

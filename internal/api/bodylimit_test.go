package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jjkroell/ridgeline/internal/store"
)

// TestBodyLimit confirms that an oversized request body is refused before a
// handler can buffer it: the limitBody middleware caps r.Body at maxRequestBody,
// so json.Decode fails and register returns a 4xx rather than reading it all.
func TestBodyLimit(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	srv := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// A body well past the 64 KB cap (a valid-looking JSON envelope whose display
	// name is enormous). It must not succeed.
	huge := `{"email":"a@b.com","password":"password123","displayName":"` +
		strings.Repeat("x", maxRequestBody+1024) + `"}`
	resp, err := http.Post(ts.URL+"/api/auth/register", "application/json", strings.NewReader(huge))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("oversized register body was accepted (status %d)", resp.StatusCode)
	}

	// A normal-sized registration on the same server still works, proving the cap
	// only rejects the oversized case.
	ok := `{"email":"real@b.com","password":"password123","displayName":"Real"}`
	resp2, err := http.Post(ts.URL+"/api/auth/register", "application/json", strings.NewReader(ok))
	if err != nil {
		t.Fatalf("post ok: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("normal register rejected: status %d", resp2.StatusCode)
	}
}

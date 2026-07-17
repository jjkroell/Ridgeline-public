package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterBurstThenDenyThenRefill(t *testing.T) {
	rl := newRateLimiter(1000, 3) // 3 burst, fast refill

	for i := 0; i < 3; i++ {
		if !rl.Allow("k") {
			t.Fatalf("burst token %d should be allowed", i+1)
		}
	}
	if rl.Allow("k") {
		t.Fatal("4th immediate request should be denied (burst exhausted)")
	}
	// A different key has its own bucket.
	if !rl.Allow("other") {
		t.Fatal("an independent key should not be affected")
	}
	// After a short wait the bucket refills (1000 tok/s -> ~5 tokens in 5ms).
	time.Sleep(5 * time.Millisecond)
	if !rl.Allow("k") {
		t.Fatal("bucket should refill after waiting")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:1234"
	if got := clientIP(r); got != "10.0.0.5" {
		t.Errorf("RemoteAddr fallback = %q, want 10.0.0.5", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 70.1.2.3")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Errorf("XFF leftmost = %q, want 203.0.113.9", got)
	}
}

func TestSameOriginWS(t *testing.T) {
	mk := func(origin, host string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/live", nil)
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	if !sameOriginWS(mk("", "mesh.example")) {
		t.Error("no Origin (non-browser client) should be allowed")
	}
	if !sameOriginWS(mk("https://mesh.example", "mesh.example")) {
		t.Error("same-origin request should be allowed")
	}
	if sameOriginWS(mk("https://evil.example", "mesh.example")) {
		t.Error("cross-origin request should be denied")
	}
}

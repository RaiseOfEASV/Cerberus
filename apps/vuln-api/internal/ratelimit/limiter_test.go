package ratelimit

import (
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestAllowSlidingHour(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	l := New(5, func() time.Time { return now })
	for i := 0; i < 5; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("request %d refused", i+1)
		}
		now = now.Add(time.Minute)
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("6th request within the hour allowed")
	}
	if !l.Allow("5.6.7.8") {
		t.Fatal("limit leaked to another key")
	}
	now = now.Add(55*time.Minute + 30*time.Second) // 11:00:30 - only the 10:00 request is over an hour old
	if !l.Allow("1.2.3.4") {
		t.Fatal("not allowed again after the oldest request aged out")
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("only one slot should have freed up")
	}
}

func TestPrunesIdleKeys(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	l := New(5, func() time.Time { return now })
	for i := 0; i < 1000; i++ {
		l.Allow(netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)}).String())
	}
	now = now.Add(2 * time.Hour)
	l.Allow("9.9.9.9")
	if n := l.Len(); n > 1 {
		t.Fatalf("%d keys kept after they all went idle", n)
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.42.0.0/16")}
	cases := []struct {
		remote, xff, want string
	}{
		{"10.42.1.7:5000", "203.0.113.9", "203.0.113.9"},
		{"10.42.1.7:5000", "198.51.100.1, 203.0.113.9", "203.0.113.9"}, // right-most = what our proxy saw
		{"192.0.2.50:5000", "203.0.113.9", "192.0.2.50"},               // untrusted peer: header ignored
		{"10.42.1.7:5000", "", "10.42.1.7"},
		{"10.42.1.7:5000", "garbage", "10.42.1.7"},
		{"[2001:db8::1]:443", "", "2001:db8::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := ClientIP(r, trusted); got != c.want {
			t.Errorf("remote %s xff %q: %q, want %q", c.remote, c.xff, got, c.want)
		}
	}
}

func TestClientIPBehindCloudflare(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.42.0.0/16"), netip.MustParsePrefix("173.245.48.0/20")}
	cases := []struct {
		name, remote, xff, want string
	}{
		// Traefik pod -> vuln-api; Traefik appended the Cloudflare edge, Cloudflare the client.
		{"real client behind cloudflare", "10.42.0.9:5000", "203.0.113.9, 173.245.48.5", "203.0.113.9"},
		// The client sent its own X-Forwarded-For: Cloudflare appends the real address after it.
		{"spoofed entry left of the real client", "10.42.0.9:5000", "1.2.3.4, 198.51.100.7, 173.245.48.5", "198.51.100.7"},
		// Directly to Traefik without Cloudflare: the right-most entry is the client.
		{"no cloudflare hop", "10.42.0.9:5000", "198.51.100.8", "198.51.100.8"},
		// Every hop trusted (internal caller): fall back to the left-most address.
		{"all hops trusted", "10.42.0.9:5000", "10.42.0.3, 173.245.48.5", "10.42.0.3"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = c.remote
		r.Header.Set("X-Forwarded-For", c.xff)
		if got := ClientIP(r, trusted); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

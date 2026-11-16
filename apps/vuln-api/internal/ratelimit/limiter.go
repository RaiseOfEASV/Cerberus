// Package ratelimit limits anonymous submissions per client (D12). It is
// in-memory: one replica, and a restart resets the counters - a documented
// residual risk, acceptable for an abuse speed bump.
package ratelimit

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

type Limiter struct {
	perHour int
	now     func() time.Time
	mu      sync.Mutex
	hits    map[string][]time.Time
	sweep   time.Time
}

func New(perHour int, now func() time.Time) *Limiter {
	return &Limiter{perHour: perHour, now: now, hits: map[string][]time.Time{}}
}

// Allow records an attempt for key and reports whether it is within
// perHour attempts in the last hour (sliding window).
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-time.Hour)
	if now.Sub(l.sweep) > 10*time.Minute {
		for k, ts := range l.hits {
			if len(ts) == 0 || !ts[len(ts)-1].After(cutoff) {
				delete(l.hits, k)
			}
		}
		l.sweep = now
	}
	ts := l.hits[key]
	i := 0
	for i < len(ts) && !ts[i].After(cutoff) {
		i++
	}
	ts = ts[i:]
	if len(ts) >= l.perHour {
		l.hits[key] = ts
		return false
	}
	l.hits[key] = append(ts, now)
	return true
}

// Len is the number of tracked keys (tests).
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.hits)
}

// ClientIP is the address to rate-limit on. X-Forwarded-For is only
// believed when the direct peer is a trusted proxy (Traefik's pod range).
// It is then walked from the right, skipping every trusted hop (Traefik,
// Cloudflare's edges), and the first untrusted address is the client - an
// entry the client wrote itself sits further left and is never reached.
// If every hop is trusted, the left-most address is used.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	if !isTrusted(peer, trusted) {
		return peer.String()
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	var leftmost netip.Addr
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			break // a malformed hop ends the chain we can trust
		}
		if !isTrusted(a, trusted) {
			return a.String()
		}
		leftmost = a
	}
	if leftmost.IsValid() {
		return leftmost.String()
	}
	return peer.String()
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a.Unmap()) {
			return true
		}
	}
	return false
}

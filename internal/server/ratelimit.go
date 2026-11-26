package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// argon2Sem limits concurrent Argon2id password verifications (64MB each) to prevent OOM DoS.
var argon2Sem = make(chan struct{}, 4)

func acquireArgon2Slot() {
	argon2Sem <- struct{}{}
}

func releaseArgon2Slot() {
	<-argon2Sem
}

type rateLimitBucket struct {
	count     int
	resetAt   time.Time
	blockedAt time.Time
}

type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateLimitBucket
	limit   int
	window  time.Duration
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		buckets: make(map[string]*rateLimitBucket),
		limit:   limit,
		window:  window,
	}
}

func clientIPFromRequest(r *http.Request) string {
	// Use RemoteAddr directly for security-sensitive rate limiting so XFF cannot be spoofed.
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func (l *ipRateLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Periodically prune expired buckets
	if len(l.buckets) > 1000 {
		for k, b := range l.buckets {
			if now.After(b.resetAt) {
				delete(l.buckets, k)
			}
		}
	}

	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		l.buckets[key] = &rateLimitBucket{
			count:   1,
			resetAt: now.Add(l.window),
		}
		return true
	}

	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

var (
	loginRateLimiter = newIPRateLimiter(10, 5*time.Minute)
	oauthRateLimiter = newIPRateLimiter(30, time.Minute)
)

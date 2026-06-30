package middleware

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"cebupac/backend/config"
)

type tokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64
	lastRefill time.Time
}

// RateLimiter enforces IP-based token bucket rate limiting.
type RateLimiter struct {
	cfg     *config.Config
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	ttl     time.Duration
}

// NewRateLimiter builds a token bucket limiter using configuration values.
func NewRateLimiter(cfg *config.Config) *RateLimiter {
	if cfg == nil {
		cfg = config.GetConfig()
	}
	return &RateLimiter{cfg: cfg, buckets: make(map[string]*tokenBucket), ttl: 10 * time.Minute}
}

// Middleware applies rate limiting to incoming requests.
func (r *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r == nil || !r.cfg.RateLimit.Enabled {
			next.ServeHTTP(w, req)
			return
		}
		ip := clientIP(req)
		if !r.allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, req)
	})
}

func (r *RateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for ip, bucket := range r.buckets {
		if now.Sub(bucket.lastRefill) > r.ttl {
			delete(r.buckets, ip)
		}
	}
	burst := float64(maxInt(1, r.cfg.RateLimit.BurstSize))
	refillRate := float64(maxInt(1, r.cfg.RateLimit.RequestsPerMin)) / 60.0
	bucket, ok := r.buckets[key]
	if !ok {
		bucket = &tokenBucket{tokens: burst, capacity: burst, refillRate: refillRate, lastRefill: now}
		r.buckets[key] = bucket
	}
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens = math.Min(bucket.capacity, bucket.tokens+elapsed*bucket.refillRate)
	bucket.lastRefill = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func clientIP(req *http.Request) string {
	if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := req.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil {
		return host
	}
	return req.RemoteAddr
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

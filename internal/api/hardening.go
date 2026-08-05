package api

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/gongahkia/norbot/internal/config"
)

type clientLimiter struct {
	mu      sync.Mutex
	entries map[string]limiterEntry
	rate    rate.Limit
	burst   int
}

type limiterEntry struct {
	limiter *rate.Limiter
	seen    time.Time
}

func newClientLimiter(policy config.HTTPPolicy) *clientLimiter {
	perMinute, burst := policy.RatePerMinute, policy.RateBurst
	if perMinute < 1 {
		perMinute = 240
	}
	if burst < 1 {
		burst = 60
	}
	return &clientLimiter{entries: map[string]limiterEntry{}, rate: rate.Limit(float64(perMinute) / 60), burst: burst}
}

func (l *clientLimiter) allow(key string) bool {
	if key == "" {
		key = "unknown"
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.seen) > 15*time.Minute {
		entry = limiterEntry{limiter: rate.NewLimiter(l.rate, l.burst)}
	}
	entry.seen = now
	l.entries[key] = entry
	if len(l.entries) > 10000 {
		for candidate, item := range l.entries {
			if now.Sub(item.seen) > 15*time.Minute {
				delete(l.entries, candidate)
			}
		}
	}
	return entry.limiter.Allow()
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		connectSources := "'self'"
		if issuer, err := url.Parse(s.oidc.Issuer); err == nil && issuer.Scheme == "https" && issuer.Host != "" {
			connectSources += " " + issuer.Scheme + "://" + issuer.Host
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src "+connectSources+"; img-src 'self' data:; style-src 'self' 'unsafe-inline'; base-uri 'self'; frame-ancestors 'none'")
		if s.security.Public && isHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if s.security.Public && !isHTTPS(r) && r.URL.Path != "/api/health" {
			writeError(w, http.StatusUpgradeRequired, fmt.Errorf("https is required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := "ip:" + remoteIP(r)
		if principal, _ := r.Context().Value(operatorContextKey{}).(string); principal != "" {
			key = "operator:" + principal
		}
		if !s.limiter.allow(key) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, fmt.Errorf("request rate limit exceeded"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) protectMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" && s.security.Public {
			expected := strings.TrimSpace(os.Getenv(s.security.HTTP.MetricsTokenEnv))
			actual := strings.TrimSpace(r.Header.Get("X-Norbot-Metrics-Token"))
			if expected == "" || actual == "" || actual != expected {
				writeError(w, http.StatusUnauthorized, fmt.Errorf("metrics token is required"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

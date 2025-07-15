package middleware

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitMiddleware implements rate limiting using token bucket algorithm
func RateLimitMiddleware(requestsPerSecond float64, burstSize int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), burstSize)
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error": "Rate limit exceeded"}`))
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// PerIPRateLimitMiddleware implements per-IP rate limiting
func PerIPRateLimitMiddleware(requestsPerSecond float64, burstSize int) func(http.Handler) http.Handler {
	limiters := make(map[string]*rate.Limiter)
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get client IP
			clientIP := getClientIP(r)
			
			// Get or create limiter for this IP
			limiter, exists := limiters[clientIP]
			if !exists {
				limiter = rate.NewLimiter(rate.Limit(requestsPerSecond), burstSize)
				limiters[clientIP] = limiter
			}
			
			if !limiter.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error": "Rate limit exceeded"}`))
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return forwarded
	}
	
	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}
	
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// APIRateLimitMiddleware applies stricter rate limiting to API endpoints
func APIRateLimitMiddleware() func(http.Handler) http.Handler {
	return PerIPRateLimitMiddleware(10, 20) // 10 requests per second, burst of 20
}

// WebhookRateLimitMiddleware applies rate limiting to webhook endpoints
func WebhookRateLimitMiddleware() func(http.Handler) http.Handler {
	return PerIPRateLimitMiddleware(100, 200) // 100 requests per second, burst of 200
}

// CleanupRateLimiters periodically cleans up old rate limiters to prevent memory leaks
func CleanupRateLimiters(limiters map[string]*rate.Limiter, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for range ticker.C {
		// Simple cleanup - remove limiters that haven't been used recently
		// In production, you'd want more sophisticated cleanup logic
		for ip, limiter := range limiters {
			// If limiter has full tokens, it hasn't been used recently
			if limiter.Tokens() == float64(limiter.Burst()) {
				delete(limiters, ip)
			}
		}
	}
}
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"knowthis/internal/metrics"
)

// MetricsMiddleware records HTTP metrics
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create a response writer that captures status code
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		
		// Call the next handler
		next.ServeHTTP(rw, r)
		
		// Record metrics
		duration := time.Since(start)
		statusCode := strconv.Itoa(rw.statusCode)
		
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, statusCode).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration.Seconds())
	})
}
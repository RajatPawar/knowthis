package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// LoggingMiddleware logs HTTP requests with structured logging
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Generate request ID
		requestID := uuid.New().String()
		
		// Create a response writer that captures status code
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		
		// Add request ID to headers
		rw.Header().Set("X-Request-ID", requestID)
		
		// Call the next handler
		next.ServeHTTP(rw, r)
		
		// Log the request
		slog.Info("HTTP Request",
			slog.String("request_id", requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
			slog.Int("status_code", rw.statusCode),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
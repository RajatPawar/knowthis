package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// SetupLogger configures structured logging for the application
func SetupLogger() *slog.Logger {
	// Default to INFO level
	level := slog.LevelInfo
	
	// Parse log level from environment
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		switch strings.ToUpper(envLevel) {
		case "DEBUG":
			level = slog.LevelDebug
		case "INFO":
			level = slog.LevelInfo
		case "WARN":
			level = slog.LevelWarn
		case "ERROR":
			level = slog.LevelError
		}
	}

	// Create handler with appropriate format
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
			AddSource: true,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
			AddSource: true,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	
	return logger
}

// ContextWithLogger adds a logger to the context
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, "logger", logger)
}

// LoggerFromContext retrieves the logger from context, falls back to default
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value("logger").(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// RequestLogger creates a logger with request-specific fields
func RequestLogger(ctx context.Context, requestID, method, path string) *slog.Logger {
	logger := LoggerFromContext(ctx)
	return logger.With(
		slog.String("request_id", requestID),
		slog.String("method", method),
		slog.String("path", path),
	)
}
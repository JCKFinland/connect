package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// New creates and configures a structured logger.
//
// Development:
//   - Human-readable text logs
//
// Production:
//   - JSON logs
func New(level string, environment string) *slog.Logger {

	var logLevel slog.Level

	switch strings.ToLower(level) {

	case "debug":
		logLevel = slog.LevelDebug

	case "warn":
		logLevel = slog.LevelWarn

	case "error":
		logLevel = slog.LevelError

	default:
		logLevel = slog.LevelInfo
	}

	options := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler

	if environment == "production" {

		handler = slog.NewJSONHandler(os.Stdout, options)

	} else {

		handler = slog.NewTextHandler(os.Stdout, options)
	}

	logger := slog.New(handler)

	slog.SetDefault(logger)

	return logger
}

// WithRequestID returns a logger that includes the request ID.
func WithRequestID(log *slog.Logger, requestID string) *slog.Logger {

	return log.With(
		slog.String("request_id", requestID),
	)
}

// WithUserID returns a logger that includes the authenticated user ID.
func WithUserID(log *slog.Logger, userID string) *slog.Logger {

	return log.With(
		slog.String("user_id", userID),
	)
}

// Info logs an informational message.
func Info(ctx context.Context, message string, attrs ...any) {
	slog.InfoContext(ctx, message, attrs...)
}

// Error logs an error message.
func Error(ctx context.Context, message string, attrs ...any) {
	slog.ErrorContext(ctx, message, attrs...)
}

// Warn logs a warning message.
func Warn(ctx context.Context, message string, attrs ...any) {
	slog.WarnContext(ctx, message, attrs...)
}

// Debug logs a debug message.
func Debug(ctx context.Context, message string, attrs ...any) {
	slog.DebugContext(ctx, message, attrs...)
}
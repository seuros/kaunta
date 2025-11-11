package logging

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	initOnce sync.Once
	logger   *slog.Logger
)

// L returns the shared application logger, initializing it on first use.
func L() *slog.Logger {
	initOnce.Do(func() {
		logger = slog.New(newHandler())
	})
	return logger
}

func newHandler() slog.Handler {
	level := parseLevel(os.Getenv("KAUNTA_LOG_LEVEL"))
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: strings.EqualFold(os.Getenv("KAUNTA_LOG_SOURCE"), "true"),
	}

	switch strings.ToLower(os.Getenv("KAUNTA_LOG_FORMAT")) {
	case "json", "structured":
		return slog.NewJSONHandler(os.Stdout, opts)
	default:
		// Text handler writes to stderr so JSON output remains clean if enabled later.
		return slog.NewTextHandler(os.Stderr, opts)
	}
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// With returns a child logger with additional attributes.
func With(args ...any) *slog.Logger {
	return L().With(args...)
}

// Fatal logs the message at error level and exits with status 1.
func Fatal(msg string, args ...any) {
	L().Error(msg, args...)
	os.Exit(1)
}

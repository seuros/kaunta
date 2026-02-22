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
	exitFunc = os.Exit
)

// L returns the shared application logger, initializing it on first use.
func L() *slog.Logger {
	initOnce.Do(func() {
		logger = newLogger()
	})
	return logger
}

// Sync is a no-op for slog (flushes automatically).
func Sync() error {
	return nil
}

func newLogger() *slog.Logger {
	level := parseLevel(os.Getenv("KAUNTA_LOG_LEVEL"))
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: strings.EqualFold(os.Getenv("KAUNTA_LOG_SOURCE"), "true"),
	}

	format := strings.ToLower(os.Getenv("KAUNTA_LOG_FORMAT"))
	var handler slog.Handler
	if format == "json" || format == "structured" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
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

// Fatal logs the message at error level and exits with status 1.
func Fatal(msg string, args ...any) {
	L().Error(msg, args...)
	exitFunc(1)
}

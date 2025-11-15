package logging

import (
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	initOnce sync.Once
	logger   *zap.Logger
	exitFunc = os.Exit
)

// L returns the shared application logger, initializing it on first use.
func L() *zap.Logger {
	initOnce.Do(func() {
		logger = newLogger()
	})
	return logger
}

// Sync flushes any buffered log entries
func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}

func newLogger() *zap.Logger {
	config := zap.NewProductionConfig()

	// Parse log level from environment
	level := parseLevel(os.Getenv("KAUNTA_LOG_LEVEL"))
	config.Level = zap.NewAtomicLevelAt(level)

	// Configure encoder based on format
	format := strings.ToLower(os.Getenv("KAUNTA_LOG_FORMAT"))
	if format == "json" || format == "structured" {
		config.Encoding = "json"
	} else {
		config.Encoding = "console"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Add source location if requested
	if strings.EqualFold(os.Getenv("KAUNTA_LOG_SOURCE"), "true") {
		config.Development = true
	}

	// Output to stderr for consistency with slog behavior
	config.OutputPaths = []string{"stderr"}
	config.ErrorOutputPaths = []string{"stderr"}

	logger, err := config.Build()
	if err != nil {
		// Fallback to development logger if config fails
		logger, _ = zap.NewDevelopment()
	}

	return logger
}

func parseLevel(value string) zapcore.Level {
	switch strings.ToLower(value) {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Fatal logs the message at error level and exits with status 1.
func Fatal(msg string, fields ...zap.Field) {
	L().Error(msg, fields...)
	exitFunc(1)
}

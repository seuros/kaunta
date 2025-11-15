package logging

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func resetLoggerForTest() {
	initOnce = sync.Once{}
	logger = nil
	exitFunc = os.Exit
}

func TestParseLevelMappings(t *testing.T) {
	assert.Equal(t, zapcore.DebugLevel, parseLevel("debug"))
	assert.Equal(t, zapcore.WarnLevel, parseLevel("warn"))
	assert.Equal(t, zapcore.WarnLevel, parseLevel("warning"))
	assert.Equal(t, zapcore.ErrorLevel, parseLevel("error"))
	assert.Equal(t, zapcore.InfoLevel, parseLevel("unknown"))
}

func TestLoggerSingleton(t *testing.T) {
	resetLoggerForTest()
	first := L()
	second := L()
	assert.Same(t, first, second)
}

func TestFatalInvokesExitFunction(t *testing.T) {
	resetLoggerForTest()

	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	// Replace logger with one writing to /dev/null to avoid noisy output
	logger = zap.NewNop()
	initOnce = sync.Once{} // prevent L() from reinitializing
	initOnce.Do(func() {}) // mark as done so L() uses existing logger

	Fatal("boom", zap.String("key", "value"))

	require.Equal(t, 1, exitCode)
}

func TestSync(t *testing.T) {
	resetLoggerForTest()

	// Test sync with nil logger
	assert.Nil(t, Sync())

	// Test sync with initialized logger
	// Note: Sync() may return error on stderr/stdout which is expected
	L()
	_ = Sync() // Error is acceptable for stderr
}

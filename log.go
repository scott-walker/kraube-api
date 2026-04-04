package kraube

import (
	"log/slog"
	"os"
)

// logger is the package-level logger. nil = silent (production default).
var logger *slog.Logger

// SetLogger sets the package-level logger. Pass nil to disable logging.
func SetLogger(l *slog.Logger) {
	logger = l
}

// EnableDevLog enables verbose stderr logging for development.
// Call once at startup: kraube.EnableDevLog()
func EnableDevLog() {
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// log helpers — all no-op when logger is nil.

func logDebug(msg string, args ...any) {
	if logger != nil {
		logger.Debug(msg, args...)
	}
}

func logInfo(msg string, args ...any) {
	if logger != nil {
		logger.Info(msg, args...)
	}
}

func logWarn(msg string, args ...any) {
	if logger != nil {
		logger.Warn(msg, args...)
	}
}

func logError(msg string, args ...any) {
	if logger != nil {
		logger.Error(msg, args...)
	}
}

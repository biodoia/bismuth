// Package logger provides a centralized structured logger for bismuth.
//
// Uses Go stdlib log/slog. All packages should use this instead of
// log.Printf or fmt.Fprintf(os.Stderr, ...).
//
// Initialize once at startup with logger.Init("bismuth", "json"|"text").
// Then use logger.Info("msg", "key", value, ...).
package logger

import (
	"log/slog"
	"os"
)

var L *slog.Logger

func init() {
	// Default: text handler to stderr, info level.
	L = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// Init configures the global logger. format is "json" or "text".
func Init(service, format string) {
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	switch format {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, opts)
	default:
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	L = slog.New(h).With("svc", service)
	slog.SetDefault(L)
}

// SetLevel changes the log level dynamically.
func SetLevel(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: l}
	switch h := L.Handler().(type) {
	case *slog.JSONHandler:
		L = slog.New(slog.NewJSONHandler(os.Stderr, opts)).With("svc", "bismuth")
	case *slog.TextHandler:
		L = slog.New(slog.NewTextHandler(os.Stderr, opts)).With("svc", "bismuth")
	default:
		_ = h
	}
}

// Package-level convenience functions.

func Debug(msg string, args ...any)  { L.Debug(msg, args...) }
func Info(msg string, args ...any)   { L.Info(msg, args...) }
func Warn(msg string, args ...any)   { L.Warn(msg, args...) }
func Error(msg string, args ...any)  { L.Error(msg, args...) }

// With returns a child logger with additional key-value pairs.
func With(args ...any) *slog.Logger { return L.With(args...) }

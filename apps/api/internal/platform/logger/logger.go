package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

func New() *slog.Logger {
	return NewWithWriter(os.Stdout)
}

func NewWithWriter(w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	options := &slog.HandlerOptions{
		Level:     parseLevel(os.Getenv("LOG_LEVEL")),
		AddSource: parseBool(os.Getenv("LOG_CALLER")),
	}
	var handler slog.Handler
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LOG_FORMAT")), "text") {
		handler = slog.NewTextHandler(w, options)
	} else {
		handler = slog.NewJSONHandler(w, options)
	}
	return slog.New(NewContextHandler(handler))
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
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

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

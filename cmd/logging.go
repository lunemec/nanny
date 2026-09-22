package cmd

import (
	"io"
	"log/slog"
	"strings"
)

func configureLogging(logxi string, output io.Writer) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: parseLogLevel(logxi),
	})))
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "*":
		return slog.LevelDebug
	case "*=INF":
		return slog.LevelInfo
	case "*=WRN":
		return slog.LevelWarn
	case "*=ERR":
		return slog.LevelError
	default:
		return slog.LevelError
	}
}
